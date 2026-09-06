package tui

import (
	"fmt"
	"strings"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// modal is one entry of the root's modal stack. The top modal takes every
// key; every modal sees every broadcast message, so one can close on its
// own async result wherever it sits.
type modal interface {
	// Update handles a key while on top, or a broadcast. done asks the root
	// to drop the modal from the stack.
	Update(msg tea.Msg) (done bool, cmd tea.Cmd)
	// View is the panel only; the root places it.
	View(lay layout, st styles) string
	// Keys enables and relabels the bindings the modal understands, and sets
	// k.short.
	Keys(k *keyMap)
	// Resize follows the terminal: the help viewport's height, the review
	// textarea's width.
	Resize(lay layout)
}

// ── Confirm ────────────────────────────────────────────────────────────────

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

// confirmModal guards the library keys that are hard to walk back from. It
// holds the operation; yes hands it back to the root to run.
type confirmModal struct {
	c     confirmState
	onYes tea.Cmd
}

func newConfirmModal(message string, onYes tea.Cmd) *confirmModal {
	return &confirmModal{c: newConfirmState(message), onYes: onYes}
}

func (c *confirmModal) Update(msg tea.Msg) (bool, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, nil
	}
	confirmed, handled := c.c.handleKey(keyMsg, keysFor(c))
	if !handled || c.c.Active {
		// An unknown key, or the cursor only moved between the buttons.
		return false, nil
	}
	if !confirmed {
		return true, request(reqToast{toastInfo, "Cancelled"})
	}
	return true, request(reqRunOp{cmd: c.onYes})
}

func (c *confirmModal) View(lay layout, st styles) string {
	return renderConfirmModal(c.c, max(36, min(60, lay.W-10)), st)
}

func (c *confirmModal) Keys(k *keyMap) {
	k.Select.SetHelp("↵", "choose")
	enable(&k.ConfirmYes, &k.ConfirmNo, &k.ConfirmLeft, &k.ConfirmRight, &k.Select)
	k.short = []key.Binding{k.ConfirmYes, hintAs("n/Esc", "no", k.ConfirmNo), hint("pick", k.ConfirmLeft, k.ConfirmRight)}
}

func (c *confirmModal) Resize(layout) {}

// ── Page prompt ────────────────────────────────────────────────────────────

// pageModal is the page prompt for one book. The input starts empty, so an
// accidental Enter cannot rewrite the progress, and keeps its format hint as
// the placeholder: the title and the current page get lines of their own.
// It closes on its own result, told apart from any other progress update by
// the token.
type pageModal struct {
	bookID  int
	title   string
	current int
	total   int
	input   textinput.Model
	token   int
}

func newPageModal(sh *shared, st styles, b model.UserBook) *pageModal {
	// The input sits inside a modal, so it carries the panel's background: a
	// style that only set a foreground would leave the row on the terminal's
	// own colour.
	in := textinput.New()
	in.Placeholder = "370 or +10 or -5"
	in.CharLimit = 32
	in.Prompt = "› "
	in.PromptStyle = st.modalKey
	in.TextStyle = st.modalValue
	in.PlaceholderStyle = st.modalDim
	in.Focus()

	return &pageModal{
		bookID:  b.Book.ID,
		title:   b.Book.Title,
		current: currentPage(b),
		total:   b.Book.Pages,
		input:   in,
		token:   sh.nextToken(),
	}
}

func (p *pageModal) Update(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		k := keysFor(p)
		switch {
		case key.Matches(msg, k.Back):
			return true, nil
		case key.Matches(msg, k.Select):
			raw := strings.TrimSpace(p.input.Value())
			if raw == "" {
				return false, request(reqToast{toastError, "page value cannot be empty"})
			}
			return false, request(reqSetPage{
				bookID: p.bookID, title: p.title, prevPage: p.current, raw: raw, token: p.token,
			})
		}
	case opDoneMsg:
		if msg.op == opProgress && msg.seq == p.token {
			return true, nil
		}
		return false, nil
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return false, cmd
}

// View renders the prompt as a centred panel: the book, where it stands and
// the input. It used to be printed under the layout, which cost the panes
// two rows whenever it was up.
func (p *pageModal) View(lay layout, st styles) string {
	current := fmt.Sprintf("current: page %d", p.current)
	if p.total > 0 {
		current = fmt.Sprintf("current: %d/%d", p.current, p.total)
	}
	content := strings.Join([]string{
		st.modalValue.Render(p.title),
		st.modalDim.Render(current),
		p.input.View(),
		"",
		st.modalDim.Render("Enter save · Esc cancel"),
	}, "\n")
	return renderModalPanel("Update page", content, max(36, min(60, lay.W-10)), st)
}

func (p *pageModal) Keys(k *keyMap) {
	k.Back.SetHelp("Esc", "cancel")
	k.Select.SetHelp("↵", "save")
	enable(&k.Back, &k.Select)
	k.short = []key.Binding{k.Select, k.Back}
}

func (p *pageModal) Resize(layout) {}

// ── Review / rating ────────────────────────────────────────────────────────

type reviewFocus int

const (
	reviewFocusRating reviewFocus = iota
	reviewFocusText
)

// The focused field carries a marker in its label as well as the cursor, so
// the focus is visible on a terminal without colour.
const (
	reviewFieldFocused = "▸ "
	reviewFieldBlurred = "  "
)

// reviewModal edits a book's rating and review. It stays open until its own
// save succeeds, so a failed save never throws away what was typed; the
// error is shown inside, because the status bar sits behind the overlay.
type reviewModal struct {
	book   model.UserBook
	rating textinput.Model
	text   textarea.Model
	focus  reviewFocus
	token  int
	// submitting makes the fields read-only until the save reports back.
	submitting bool
	err        string
}

func newReviewModal(sh *shared, st styles, book model.UserBook) *reviewModal {
	// The review fields sit inside a modal, so they carry its background: a
	// style that only sets a foreground would leave the field on the
	// terminal's own colour.
	rating := textinput.New()
	rating.Placeholder = "4.5"
	rating.CharLimit = 4
	rating.Prompt = reviewFieldFocused + "Rating: "
	rating.PromptStyle = st.modalKey
	rating.TextStyle = st.modalValue
	rating.PlaceholderStyle = st.modalDim

	text := textarea.New()
	text.Placeholder = "Write your review..."
	text.SetWidth(60)
	text.SetHeight(8)
	text.ShowLineNumbers = false
	for _, fs := range []*textarea.Style{&text.FocusedStyle, &text.BlurredStyle} {
		fs.Base = st.modalBg
		fs.Text = st.modalValue
		fs.Placeholder = st.modalDim
		fs.Prompt = st.modalKey
		fs.CursorLine = st.modalBg
		fs.EndOfBuffer = st.modalBg
	}

	if book.Rating > 0 {
		rating.SetValue(fmt.Sprintf("%.1f", book.Rating))
	}
	text.SetValue(book.Review)

	r := &reviewModal{book: book, rating: rating, text: text, token: sh.nextToken()}
	r.focusRating()
	return r
}

func (r *reviewModal) focusRating() {
	r.focus = reviewFocusRating
	r.rating.Prompt = reviewFieldFocused + "Rating: "
	r.rating.Focus()
	r.text.Blur()
}

func (r *reviewModal) focusText() {
	r.focus = reviewFocusText
	r.rating.Prompt = reviewFieldBlurred + "Rating: "
	r.rating.Blur()
	r.text.Focus()
}

func (r *reviewModal) Update(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return r.handleKey(msg)
	case opDoneMsg:
		if msg.op != opReview || msg.seq != r.token {
			return false, nil
		}
		r.submitting = false
		if msg.err != nil {
			// Shown in the overlay; the rating and review text are preserved.
			r.err = msg.err.Error()
			return false, nil
		}
		return true, nil
	}
	return false, r.updateField(msg)
}

func (r *reviewModal) handleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	k := keysFor(r)
	if r.submitting {
		// The fields are read-only until the save reports back; cancelling
		// drops the modal, so the pending result is reported like any other.
		if key.Matches(msg, k.Back) {
			return true, request(reqToast{toastInfo, "Review update cancelled"})
		}
		return false, nil
	}

	switch {
	case key.Matches(msg, k.Back):
		return true, request(reqToast{toastInfo, "Review update cancelled"})
	case key.Matches(msg, k.ReviewNextField):
		if r.focus == reviewFocusRating {
			r.focusText()
		} else {
			r.focusRating()
		}
		return false, nil
	case key.Matches(msg, k.ReviewPrevField):
		if r.focus == reviewFocusText {
			r.focusRating()
		} else {
			r.focusText()
		}
		return false, nil
	case key.Matches(msg, k.ReviewSave):
		rating, err := model.ParseRating(r.rating.Value())
		if err != nil {
			r.err = err.Error()
			return false, nil
		}
		r.err = ""
		r.submitting = true
		return false, request(reqReview{
			bookID: r.book.Book.ID, rating: rating, review: r.text.Value(), token: r.token,
		})
	}
	return false, r.updateField(msg)
}

// updateField forwards a message to the focused field.
func (r *reviewModal) updateField(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	if r.focus == reviewFocusRating {
		r.rating, cmd = r.rating.Update(msg)
	} else {
		r.text, cmd = r.text.Update(msg)
	}
	return cmd
}

func (r *reviewModal) View(lay layout, st styles) string {
	rating, err := model.ParseRating(r.rating.Value())
	ratingLabel := "invalid rating"
	stars := "☆☆☆☆☆"
	if err == nil {
		stars = model.StarString(rating)
		ratingLabel = fmt.Sprintf("%.1f", rating)
	}

	var sb strings.Builder
	sb.WriteString(st.modalValue.Render(r.book.Book.Title))
	sb.WriteString("\n")
	sb.WriteString(st.modalDim.Render(fallback(r.book.Book.AuthorString(), "Unknown author")))
	sb.WriteString("\n\n")
	sb.WriteString(r.rating.View())
	sb.WriteString(st.modalBg.Render("  "))
	sb.WriteString(st.modalKey.Render(stars))
	sb.WriteString(st.modalBg.Render(" "))
	sb.WriteString(st.modalDim.Render("(" + ratingLabel + ")"))
	sb.WriteString("\n\n")
	reviewMarker := reviewFieldBlurred
	if r.focus == reviewFocusText {
		reviewMarker = reviewFieldFocused
	}
	sb.WriteString(st.modalLabel.Render(reviewMarker + "Review"))
	sb.WriteString("\n")
	sb.WriteString(r.text.View())
	sb.WriteString("\n\n")
	switch {
	case r.submitting:
		sb.WriteString(st.modalDim.Render("Saving..."))
		sb.WriteString("\n")
	case r.err != "":
		// The status bar sits behind the modal, so surface the failure here.
		sb.WriteString(st.modalError.Render(r.err))
		sb.WriteString("\n")
	}

	sb.WriteString(st.modalDim.Render("Tab/Shift+Tab switch fields   Ctrl+S save   Esc cancel"))

	width := max(70, lay.W-10)
	if width > 100 {
		width = 100
	}
	return renderModalPanel("Review / Rate Book", sb.String(), width, st)
}

func (r *reviewModal) Keys(k *keyMap) {
	k.Back.SetHelp("Esc", "cancel")
	enable(&k.Back)
	if !r.submitting {
		enable(&k.ReviewSave, &k.ReviewNextField, &k.ReviewPrevField)
	}
	k.short = []key.Binding{hint("switch field", k.ReviewNextField, k.ReviewPrevField), k.ReviewSave, k.Back}
}

func (r *reviewModal) Resize(lay layout) {
	if lay.W > 0 {
		r.text.SetWidth(max(40, lay.W/2))
	}
}

// ── Timer picker ───────────────────────────────────────────────────────────

// timerPickerModal chooses which Reading book a timer starts for. It is
// drawn in the Timer pane, where the choice was always made.
type timerPickerModal struct {
	sh  *shared
	idx int
}

func newTimerPickerModal(sh *shared, idx int) *timerPickerModal {
	return &timerPickerModal{sh: sh, idx: idx}
}

func (p *timerPickerModal) Update(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return p.handleKey(msg)
	case timerOpDoneMsg:
		return true, nil
	case dataChangedMsg:
		// Background sync can shrink the Reading list while the picker is
		// open; a timer started elsewhere makes it moot.
		if msg.kind == dataLibrary && p.idx >= len(p.sh.reading) {
			p.idx = max(0, len(p.sh.reading)-1)
		}
		if msg.kind == dataLocal && p.sh.timer != nil {
			return true, nil
		}
	}
	return false, nil
}

func (p *timerPickerModal) handleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	k := keysFor(p)
	switch {
	case key.Matches(msg, k.Quit):
		return false, tea.Quit
	case key.Matches(msg, k.Help):
		return false, request(reqHelp{})
	case key.Matches(msg, k.Back):
		return true, request(reqToast{toastInfo, "Timer start cancelled"})
	case key.Matches(msg, k.Down):
		if p.idx < len(p.sh.reading)-1 {
			p.idx++
		}
	case key.Matches(msg, k.Up):
		if p.idx > 0 {
			p.idx--
		}
	case key.Matches(msg, k.Select):
		if len(p.sh.reading) == 0 {
			return true, request(reqToast{toastError, "no currently reading books available"})
		}
		if p.idx >= len(p.sh.reading) {
			p.idx = len(p.sh.reading) - 1
		}
		return true, request(reqTimer{start: true, bookID: p.sh.reading[p.idx].Book.ID})
	}
	return false, nil
}

// View renders the picker as a centred panel. Its width comes from the
// layout, like every other modal's: the pane it used to be drawn inside is
// not where the choice is made any more.
func (p *timerPickerModal) View(lay layout, st styles) string {
	width := max(40, min(64, lay.W-10))
	inner := width - 6

	var sb strings.Builder
	sb.WriteString(st.modalDim.Render("Select a book, Enter starts the timer"))
	sb.WriteString("\n\n")

	if len(p.sh.reading) == 0 {
		sb.WriteString(st.modalDim.Render("No books in Reading."))
		return renderModalPanel("Reading Timer", sb.String(), width, st)
	}

	for i, b := range p.sh.reading {
		if i >= 9 {
			break
		}
		prefix, titleStyle := "  ", st.modalValue
		if i == p.idx {
			prefix, titleStyle = "▸ ", st.modalKey
		}
		author := b.Book.AuthorString()
		if author == "" {
			author = "Unknown author"
		}
		sb.WriteString(titleStyle.Render(ansi.Truncate(prefix+b.Book.Title, inner, "…")))
		sb.WriteString("\n")
		sb.WriteString(st.modalDim.Render(ansi.Truncate("  "+author, inner, "…")))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(st.modalDim.Render("j/k move · Enter start · Esc cancel"))
	return renderModalPanel("Reading Timer", sb.String(), width, st)
}

func (p *timerPickerModal) Keys(k *keyMap) {
	k.Up.SetHelp("k", "choose")
	k.Down.SetHelp("j", "choose")
	k.Select.SetHelp("↵", "start")
	k.Back.SetHelp("Esc", "cancel")
	enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.Select, &k.Back)
	k.short = []key.Binding{k.Help, hint("choose", k.Down, k.Up), k.Select, k.Back, k.Quit}
}

func (p *timerPickerModal) Resize(layout) {}

// ── Help ───────────────────────────────────────────────────────────────────

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

// helpModal lists every key, group by group, from the same bindings the
// handlers dispatch on. keys is the keymap of the focus under the modal,
// read at render time so the rows follow it.
type helpModal struct {
	keys func() keyMap
	// version is the build the dashboard is running, which the footer names:
	// the one place it is on screen now that the intro page is gone.
	version string
	st      styles
	// vp scrolls the body, which is taller than a short terminal.
	vp viewport.Model
}

func newHelpModal(keys func() keyMap, version string, st styles) *helpModal {
	return &helpModal{keys: keys, version: version, st: st}
}

func (h *helpModal) Update(msg tea.Msg) (bool, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, nil
	}
	k := keysFor(h)
	switch {
	case key.Matches(keyMsg, k.Help, k.Back):
		return true, nil
	case key.Matches(keyMsg, k.Quit):
		return false, tea.Quit
	case key.Matches(keyMsg, k.Down):
		h.vp.LineDown(1)
	case key.Matches(keyMsg, k.Up):
		h.vp.LineUp(1)
	case key.Matches(keyMsg, k.HalfPageDown):
		h.vp.HalfViewDown()
	case key.Matches(keyMsg, k.HalfPageUp):
		h.vp.HalfViewUp()
	case key.Matches(keyMsg, k.ScrollTop):
		h.vp.GotoTop()
	case key.Matches(keyMsg, k.ScrollBottom):
		h.vp.GotoBottom()
	}
	return false, nil
}

// Resize fits the help body to the terminal. The body is taller than a
// 40-row window, so it scrolls rather than spilling off the screen.
func (h *helpModal) Resize(lay layout) {
	body := h.body()
	height := lipgloss.Height(body)
	if lay.H > 0 {
		height = min(height, max(helpModalMinBodyRows, lay.H-helpModalChromeRows-helpModalMarginRows))
	}

	offset := h.vp.YOffset
	h.vp = viewport.New(helpModalWidth-h.st.helpModal.GetHorizontalPadding(), height)
	h.vp.SetContent(body)
	h.vp.SetYOffset(offset)
}

func (h *helpModal) View(_ layout, st styles) string {
	footer := "? or esc close"
	if h.vp.TotalLineCount() > h.vp.Height {
		footer = "j/k scroll · ? or esc close"
	}
	if v := strings.TrimSpace(h.version); v != "" {
		footer += "   oku " + v
	}
	return renderModalPanel(
		"Help",
		h.rows()+"\n"+st.modalDim.Render(footer),
		helpModalWidth,
		st,
	)
}

// rows returns the rows the help body currently shows. They are taken from
// the body rather than from viewport.View, which pads its own output with
// unstyled spaces: that padding would leave the panel striped, because only
// the modal style's own fill carries its background.
func (h *helpModal) rows() string {
	lines := strings.Split(h.body(), "\n")
	height := max(1, h.vp.Height)
	start := clampInt(h.vp.YOffset, 0, max(0, len(lines)-height))

	rows := make([]string, 0, height)
	rows = append(rows, lines[start:min(len(lines), start+height)]...)
	for len(rows) < height {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

// body lists every key, group by group. The keys the focus behind the modal
// understands are drawn in full and their groups come first; the rest are
// dimmed, so the modal still teaches what the other sections can do.
func (h *helpModal) body() string {
	st := h.st
	groups := h.keys().helpGroups()
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
			keySt, descSt := st.modalKey, st.modalDesc
			if !b.Enabled() {
				keySt, descSt = st.modalDim.Bold(true), st.modalDim
			}
			rows += st.modalBg.Render("  ") +
				keySt.Width(12).Render(b.Help().Key) +
				st.modalBg.Render("  ") +
				descSt.Render(b.Help().Desc) + "\n"
		}
		title := st.modalHead
		if !g.hasEnabled() {
			title = st.modalDim.Bold(true)
		}
		sections = append(sections, title.Render(g.title)+"\n"+rows)
	}

	// Joined by hand: lipgloss.JoinVertical pads every row out to the widest
	// one with unstyled spaces, and those spaces would show as bands of
	// terminal background across the modal.
	return strings.TrimRight(strings.Join(sections, "\n"), "\n")
}

func (h *helpModal) Keys(k *keyMap) {
	k.Up.SetHelp("k", "scroll")
	k.Down.SetHelp("j", "scroll")
	k.Back.SetHelp("Esc", "close")
	enable(&k.Help, &k.Back, &k.Quit, &k.Up, &k.Down,
		&k.HalfPageUp, &k.HalfPageDown, &k.ScrollTop, &k.ScrollBottom)
	k.short = []key.Binding{hint("scroll", k.Down, k.Up), k.Back}
}

// ── Panels ─────────────────────────────────────────────────────────────────

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

// overlayModal centres a panel over a blank screen. True compositing over
// the dashboard needs lipgloss v2; on v1 the blanked background is what
// ships.
func overlayModal(lay layout, panel string) string {
	return lipgloss.Place(
		lay.W, lay.H,
		lipgloss.Center, lipgloss.Center,
		panel,
		lipgloss.WithWhitespaceChars(" "),
	)
}
