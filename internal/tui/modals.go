package tui

import (
	"fmt"
	"strings"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmState is a reusable, keyboard-driven modal confirmation state.
type confirmState struct {
	Active      bool
	Message     string
	ConfirmText string
	CancelText  string
	Cursor      int // 0=confirm, 1=cancel
}

func newConfirmState(message string) confirmState {
	return confirmState{
		Active:      true,
		Message:     message,
		ConfirmText: "Confirm",
		CancelText:  "Cancel",
		Cursor:      1,
	}
}

// handleKey answers the question with the confirm bindings of k: yes and no
// close it, the arrows move between the buttons, and Select takes the one
// under the cursor.
func (c *confirmState) handleKey(msg tea.KeyMsg, k keyMap) (confirmed bool, handled bool) {
	switch {
	case key.Matches(msg, k.ConfirmNo):
		c.Active = false
		return false, true
	case key.Matches(msg, k.ConfirmLeft):
		c.Cursor = 0
		return false, true
	case key.Matches(msg, k.ConfirmRight):
		c.Cursor = 1
		return false, true
	case key.Matches(msg, k.ConfirmYes):
		c.Active = false
		return true, true
	case key.Matches(msg, k.Select):
		c.Active = false
		return c.Cursor == 0, true
	}
	return false, false
}

func renderConfirmModal(c confirmState, width int, st styles) string {
	if width <= 0 {
		width = 50
	}
	if width < 36 {
		width = 36
	}

	// The chosen button is marked as well as coloured, so the choice is
	// visible on a terminal without colour.
	left := st.idleButton.Render("  " + c.ConfirmText)
	right := st.idleButton.Render("  " + c.CancelText)
	if c.Cursor == 0 {
		left = st.confirmButton.Render("▸ " + c.ConfirmText)
	} else {
		right = st.cancelButton.Render("▸ " + c.CancelText)
	}

	// Every part of the row carries the modal background: the gap between the
	// buttons and the space either side of them included, or a black band
	// shows through the charcoal panel.
	buttons := st.modalBg.
		Width(width - 6).
		Align(lipgloss.Center).
		Render(lipgloss.JoinHorizontal(lipgloss.Center, left, st.modalBg.Render("  "), right))

	content := st.modalValue.Render(c.Message) + "\n\n" +
		buttons + "\n\n" +
		st.modalDim.Render("y/n or Enter/Esc")
	return renderModalPanel("Confirm", content, width, st)
}

// ── View & Focus types ─────────────────────────────────────────────────────

type viewMode int

const (
	modeLibrary viewMode = iota
	modeUpdatePage
	modeReviewRating
)

// pagePromptRows is how many rows View spends on the page-update prompt it
// draws under the layout: the book, where it stands, and the input.
const pagePromptRows = 3

const (
	helpModalWidth = 50
	// helpModalChromeRows counts the modal rows that are not the scrollable
	// body: the title, the blank line under it, the footer, the padding and
	// the border.
	helpModalChromeRows = 7
	// helpModalMinBodyRows keeps the body scrollable on a very short terminal.
	helpModalMinBodyRows = 3
	// helpModalMarginRows keeps a row of dashboard above and below the modal,
	// so it reads as something laid over the screen rather than as the screen.
	helpModalMarginRows = 2
)

type dashboardReviewFocus int

const (
	dashboardReviewFocusRating dashboardReviewFocus = iota
	dashboardReviewFocusText
)

// updateConfirmMode answers the confirmation: y (or Enter on Confirm) runs the
// operation it is holding, n and Esc drop it.
func (m Model) updateConfirmMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	if key.Matches(msg, k.ForceQuit) {
		return m, tea.Quit
	}

	confirmed, handled := m.confirm.handleKey(msg, k)
	if !handled || m.confirm.Active {
		// An unknown key, or the cursor only moved between the buttons.
		return m, nil
	}

	pending := m.confirmCmd
	m.confirm = confirmState{}
	m.confirmCmd = nil
	if !confirmed {
		cmd := m.showToast(toastInfo, "Cancelled")
		return m, cmd
	}
	return m.startOp(pending)
}

// ── Page update mode ───────────────────────────────────────────────────────

func (m Model) updatePageMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	switch {
	case key.Matches(msg, k.Back):
		m.closePageModal()
		return m, nil
	case key.Matches(msg, k.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, k.Select):
		raw := strings.TrimSpace(m.pageInput.Value())
		if raw == "" {
			cmd := m.showToast(toastError, "page value cannot be empty")
			return m, cmd
		}
		if m.isLoading() {
			cmd := m.showToast(toastWarn, inFlightNotice)
			return m, cmd
		}
		m.pageSubmitting = true
		return m.startOp(updateProgressCmd(m.ctx, m.app, m.pendingBookID, m.pageBookTitle, m.pageCurrentPage, raw))
	}

	var cmd tea.Cmd
	m.pageInput, cmd = m.pageInput.Update(msg)
	return m, cmd
}

// openPageModal opens the page prompt for a book. The input starts empty, so
// an accidental Enter cannot rewrite the progress, and keeps its format hint
// as the placeholder: the title and the current page get lines of their own.
func (m *Model) openPageModal(b model.UserBook) {
	m.mode = modeUpdatePage
	m.pendingBookID = b.Book.ID
	m.pageBookTitle = b.Book.Title
	m.pageCurrentPage = b.CurrentPage
	if len(b.UserBookReads) > 0 {
		m.pageCurrentPage = b.UserBookReads[0].ProgressPages
	}
	m.pageTotalPages = b.Book.Pages
	m.pageInput.SetValue("")
	m.pageInput.Focus()
	// The prompt is taller than the help bar it replaces.
	m.resize()
}

func (m *Model) closePageModal() {
	m.mode = modeLibrary
	m.pageSubmitting = false
	m.pageBookTitle = ""
	m.pageCurrentPage = 0
	m.pageTotalPages = 0
	m.pageInput.Blur()
	m.pageInput.SetValue("")
	m.resize()
}

// pagePrompt renders the page-update prompt under the layout. It is always
// pagePromptRows tall, which is what the layout height is computed against.
func (m Model) pagePrompt() string {
	current := fmt.Sprintf("current: page %d", m.pageCurrentPage)
	if m.pageTotalPages > 0 {
		current = fmt.Sprintf("current: %d/%d", m.pageCurrentPage, m.pageTotalPages)
	}
	return "\n" + strings.Join([]string{
		" " + m.st.keyHint.Render("Update page") + "  " + m.st.value.Render(m.pageBookTitle),
		" " + m.st.dim.Render(current),
		" " + m.pageInput.View() + m.st.dim.Render("   Enter save · Esc cancel"),
	}, "\n")
}

// ── Review/rating mode ─────────────────────────────────────────────────────

func (m Model) updateReviewRatingMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	if m.reviewSubmitting {
		// The fields are read-only until the save reports back; cancelling
		// bumps reviewSeq, so the pending result is ignored.
		switch {
		case key.Matches(msg, k.ForceQuit):
			return m, tea.Quit
		case key.Matches(msg, k.Back):
			m.closeReviewRatingModal()
			cmd := m.showToast(toastInfo, "Review update cancelled")
			return m, cmd
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, k.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, k.Back):
		m.closeReviewRatingModal()
		cmd := m.showToast(toastInfo, "Review update cancelled")
		return m, cmd
	case key.Matches(msg, k.ReviewNextField):
		if m.reviewFocus == dashboardReviewFocusRating {
			m.focusReviewTextField()
		} else {
			m.focusReviewRatingField()
		}
		return m, nil
	case key.Matches(msg, k.ReviewPrevField):
		if m.reviewFocus == dashboardReviewFocusText {
			m.focusReviewRatingField()
		} else {
			m.focusReviewTextField()
		}
		return m, nil
	case key.Matches(msg, k.ReviewSave):
		if m.reviewBook == nil {
			return m, nil
		}
		rating, err := model.ParseRating(m.reviewRatingInput.Value())
		if err != nil {
			m.reviewErr = err.Error()
			return m, nil
		}
		review := m.reviewTextInput.Value()
		m.reviewErr = ""
		toastCmd := m.showToast(toastInfo, reviewSavePendingMessage(review))
		// The modal stays open until the save succeeds, so a failure can show
		// the error without discarding the draft.
		m.reviewSubmitting = true
		save := m.beginLoading(submitReviewRatingCmd(m.ctx, m.app, m.reviewBook.Book.ID, rating, review, m.reviewSeq))
		return m, tea.Batch(save, toastCmd)
	}

	var cmd tea.Cmd
	if m.reviewFocus == dashboardReviewFocusRating {
		m.reviewRatingInput, cmd = m.reviewRatingInput.Update(msg)
	} else {
		m.reviewTextInput, cmd = m.reviewTextInput.Update(msg)
	}
	return m, cmd
}

func (m *Model) openReviewRatingModal(book model.UserBook) {
	b := book
	m.reviewBook = &b
	m.mode = modeReviewRating
	m.showHelp = false
	m.reviewSubmitting = false
	m.reviewErr = ""
	m.reviewSeq++

	if b.Rating > 0 {
		m.reviewRatingInput.SetValue(fmt.Sprintf("%.1f", b.Rating))
	} else {
		m.reviewRatingInput.SetValue("")
	}
	m.reviewTextInput.SetValue(b.Review)
	m.focusReviewRatingField()
}

func (m *Model) closeReviewRatingModal() {
	m.mode = modeLibrary
	m.reviewBook = nil
	m.reviewSubmitting = false
	m.reviewErr = ""
	m.reviewSeq++
	m.reviewRatingInput.Blur()
	m.reviewTextInput.Blur()
}

// The focused field carries a marker in its label as well as the cursor, so
// the focus is visible on a terminal without colour.
const (
	reviewFieldFocused = "▸ "
	reviewFieldBlurred = "  "
)

func (m *Model) focusReviewRatingField() {
	m.reviewFocus = dashboardReviewFocusRating
	m.reviewRatingInput.Prompt = reviewFieldFocused + "Rating: "
	m.reviewRatingInput.Focus()
	m.reviewTextInput.Blur()
}

func (m *Model) focusReviewTextField() {
	m.reviewFocus = dashboardReviewFocusText
	m.reviewRatingInput.Prompt = reviewFieldBlurred + "Rating: "
	m.reviewRatingInput.Blur()
	m.reviewTextInput.Focus()
}

func (m Model) reviewRatingOverlay() string {
	if m.reviewBook == nil {
		return renderModalPanel("Review / Rate Book", m.st.modalDim.Render("No book selected"), 48, m.st)
	}

	rating, err := model.ParseRating(m.reviewRatingInput.Value())
	ratingLabel := "invalid rating"
	stars := "☆☆☆☆☆"
	if err == nil {
		stars = model.StarString(rating)
		ratingLabel = fmt.Sprintf("%.1f", rating)
	}

	var sb strings.Builder
	sb.WriteString(m.st.modalValue.Render(m.reviewBook.Book.Title))
	sb.WriteString("\n")
	sb.WriteString(m.st.modalDim.Render(fallback(m.reviewBook.Book.AuthorString(), "Unknown author")))
	sb.WriteString("\n\n")
	sb.WriteString(m.reviewRatingInput.View())
	sb.WriteString(m.st.modalBg.Render("  "))
	sb.WriteString(m.st.modalKey.Render(stars))
	sb.WriteString(m.st.modalBg.Render(" "))
	sb.WriteString(m.st.modalDim.Render("(" + ratingLabel + ")"))
	sb.WriteString("\n\n")
	reviewMarker := reviewFieldBlurred
	if m.reviewFocus == dashboardReviewFocusText {
		reviewMarker = reviewFieldFocused
	}
	sb.WriteString(m.st.modalLabel.Render(reviewMarker + "Review"))
	sb.WriteString("\n")
	sb.WriteString(m.reviewTextInput.View())
	sb.WriteString("\n\n")
	switch {
	case m.reviewSubmitting:
		sb.WriteString(m.st.modalDim.Render("Saving..."))
		sb.WriteString("\n")
	case m.reviewErr != "":
		// The status bar sits behind the modal, so surface the failure here.
		sb.WriteString(m.st.modalError.Render(m.reviewErr))
		sb.WriteString("\n")
	}

	sb.WriteString(m.st.modalDim.Render("Tab/Shift+Tab switch fields   Ctrl+S save   Esc cancel"))

	width := max(70, m.width-10)
	if width > 100 {
		width = 100
	}
	return renderModalPanel("Review / Rate Book", sb.String(), width, m.st)
}

// ── Help ───────────────────────────────────────────────────────────────────

// openHelp shows the help modal, scrolled back to the top.
func (m *Model) openHelp() {
	m.showHelp = true
	m.syncHelpViewport()
	m.helpViewport.GotoTop()
}

// syncHelpViewport fits the help body to the terminal. The body is taller than
// a 40-row window, so it scrolls rather than spilling off the screen.
func (m *Model) syncHelpViewport() {
	body := m.helpModalBody()
	h := lipgloss.Height(body)
	if m.height > 0 {
		h = min(h, max(helpModalMinBodyRows, m.height-helpModalChromeRows-helpModalMarginRows))
	}

	offset := m.helpViewport.YOffset
	m.helpViewport = viewport.New(helpModalWidth-m.st.helpModal.GetHorizontalPadding(), h)
	m.helpViewport.SetContent(body)
	m.helpViewport.SetYOffset(offset)
}

func (m Model) renderHelpModal() string {
	footer := "? or esc close"
	if m.helpViewport.TotalLineCount() > m.helpViewport.Height {
		footer = "j/k scroll · ? or esc close"
	}
	return renderModalPanel(
		"Help",
		m.helpModalRows()+"\n"+m.st.modalDim.Render(footer),
		helpModalWidth,
		m.st,
	)
}

// helpModalRows returns the rows the help body currently shows. They are taken
// from the body rather than from viewport.View, which pads its own output with
// unstyled spaces: that padding would leave the panel striped, because only
// the modal style's own fill carries its background.
func (m Model) helpModalRows() string {
	lines := strings.Split(m.helpModalBody(), "\n")
	h := max(1, m.helpViewport.Height)
	start := clampInt(m.helpViewport.YOffset, 0, max(0, len(lines)-h))

	rows := make([]string, 0, h)
	rows = append(rows, lines[start:min(len(lines), start+h)]...)
	for len(rows) < h {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

// helpModalBody lists every key, group by group, from the same bindings the
// handlers dispatch on. The keys the focus behind the modal understands are
// drawn in full and their groups come first; the rest are dimmed, so the
// modal still teaches what the other sections can do.
func (m Model) helpModalBody() string {
	behind := m
	behind.showHelp = false
	k := behind.activeKeys()

	groups := k.helpGroups()
	active := make([]helpGroup, 0, len(groups))
	inactive := make([]helpGroup, 0, len(groups))
	for _, g := range groups {
		if g.hasEnabled() {
			active = append(active, g)
		} else {
			inactive = append(inactive, g)
		}
	}

	sections := make([]string, 0, len(groups))
	for _, g := range append(active, inactive...) {
		rows := ""
		for _, b := range g.bindings {
			// Every run carries the modal background, including the gaps: a
			// style that only sets a foreground ends with a reset, which would
			// stripe the row with the terminal's own background.
			keySt, descSt := m.st.modalKey, m.st.modalDesc
			if !b.Enabled() {
				keySt, descSt = m.st.modalDim.Bold(true), m.st.modalDim
			}
			rows += m.st.modalBg.Render("  ") +
				keySt.Width(12).Render(b.Help().Key) +
				m.st.modalBg.Render("  ") +
				descSt.Render(b.Help().Desc) + "\n"
		}
		title := m.st.modalHead
		if !g.hasEnabled() {
			title = m.st.modalDim.Bold(true)
		}
		sections = append(sections, title.Render(g.title)+"\n"+rows)
	}

	// Joined by hand: lipgloss.JoinVertical pads every row out to the widest
	// one with unstyled spaces, and those spaces would show as bands of
	// terminal background across the modal.
	return strings.TrimRight(strings.Join(sections, "\n"), "\n")
}

func (m Model) overlayModal(modal string) string {
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		modal,
		lipgloss.WithWhitespaceChars(" "),
	)
}

func renderModalPanel(title, content string, width int, st styles) string {
	style := st.helpModal
	if width > 0 {
		style = style.Width(width)
	}

	body := content
	if strings.TrimSpace(title) != "" {
		body = st.modalTitle.Render(title) + "\n\n" + content
	}
	// The rows are left short on purpose: lipgloss fills them out to the panel
	// width with the style's own background, which is the only fill that
	// carries it. Pre-padding here would be trimmed by the wrap and refilled
	// with unstyled spaces, striping the panel.
	return style.Render(body)
}
