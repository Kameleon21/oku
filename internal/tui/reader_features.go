package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
)

type reqBookDetail struct{ selection selection }
type detailLoadedMsg struct {
	id   int
	book *model.BookDetail
	err  error
}
type journalLoadedMsg struct {
	id      int
	entries []model.BookJournalEntry
	err     error
}
type reqPausedShelf struct{}
type pausedLoadedMsg struct {
	books []model.UserBook
	err   error
}
type reqQueueMove struct{ bookID, delta int }
type reqQueueRefresh struct{}
type queueLoadedMsg struct {
	ids    []int
	bookID int
	err    error
}
type reqTrending struct{ seq int }
type reqPrivateNote struct {
	bookID, token int
	text          string
}

type reqJournal struct {
	bookID, token int
	event, text   string
}

func (m *Model) handleReaderRequest(msg tea.Msg) (tea.Cmd, bool) {
	switch r := msg.(type) {
	case reqBookDetail:
		id := 0
		if r.selection.Book != nil {
			id = r.selection.Book.BookID
		} else if r.selection.Result != nil {
			id = r.selection.Result.ID
		}
		if id == 0 {
			return m.showToast(toastWarn, app.ErrOffline.Error()), true
		}
		if cmd := m.requireOnline(); cmd != nil {
			return cmd, true
		}
		detail := bookDetailCmd(m.ctx, m.backend, id)
		if r.selection.Book != nil {
			return m.run(detail, bookJournalCmd(m.ctx, m.backend, id)), true
		}
		return m.run(detail), true
	case detailLoadedMsg:
		if r.err != nil {
			return m.showToast(toastError, r.err.Error()), true
		}
		if m.shared.details == nil {
			m.shared.details = map[int]*model.BookDetail{}
		}
		m.shared.details[r.id] = r.book
		m.detail.stamp++
		return nil, true
	case journalLoadedMsg:
		if r.err != nil {
			return m.showToast(toastError, "Load journal: "+r.err.Error()), true
		}
		if m.shared.journals == nil {
			m.shared.journals = map[int][]model.BookJournalEntry{}
		}
		m.shared.journals[r.id] = r.entries
		m.detail.stamp++
		return nil, true
	case reqPausedShelf:
		if cmd := m.requireOnline(); cmd != nil {
			return cmd, true
		}
		return m.run(pausedShelfCmd(m.ctx, m.backend)), true
	case pausedLoadedMsg:
		if r.err != nil {
			return m.showToast(toastError, r.err.Error()), true
		}
		m.shared.paused = r.books
		return m.broadcast(dataChangedMsg{dataLibrary}), true
	case reqQueueMove:
		if m.busy() {
			return m.showToast(toastWarn, inFlightNotice), true
		}
		if cmd := m.requireOnline(); cmd != nil {
			return cmd, true
		}
		return m.runWrite(queueMoveCmd(m.ctx, m.backend, r.bookID, r.delta)), true
	case reqQueueRefresh:
		if m.busy() {
			return m.showToast(toastWarn, inFlightNotice), true
		}
		if cmd := m.requireOnline(); cmd != nil {
			return cmd, true
		}
		return m.run(queueRefreshCmd(m.ctx, m.backend)), true
	case queueLoadedMsg:
		if r.err != nil {
			return m.showToast(toastError, r.err.Error()), true
		}
		m.shared.queueOrder = r.ids
		app.SortQueue(m.shared.oku, r.ids)
		cmd := m.broadcast(dataChangedMsg{dataLibrary})
		if s, ok := m.sections[tabOku].(*librarySection); ok && r.bookID > 0 {
			for i, b := range s.books() {
				if b.BookID == r.bookID {
					s.list.Select(i)
					break
				}
			}
		}
		return tea.Batch(cmd, m.showToast(toastSuccess, "Reading queue saved")), true
	case reqTrending:
		// Not refused up front: the search pane reports a failed fetch
		// itself, offline included, the way it does for a search.
		return m.run(trendingCmd(m.ctx, m.backend, r.seq)), true
	case reqPrivateNote:
		if m.busy() {
			return m.refuse(opPrivateNote, r.token), true
		}
		return m.run(savePrivateNoteCmd(m.backend, r.bookID, r.token, r.text)), true
	case reqJournal:
		if m.busy() {
			return m.refuse(opJournal, r.token), true
		}
		if !m.backend.Online() {
			// The modal is drawn over the status bar, so the refusal goes to
			// it as its own failed result rather than as a toast; it is not
			// work, so it is not counted.
			return request(opDoneMsg{op: opJournal, seq: r.token, err: app.ErrOffline}), true
		}
		return m.run(addJournalEntryCmd(m.ctx, m.backend, r.bookID, r.token, r.event, r.text)), true
	}
	return nil, false
}

// Rich metadata is appended to the existing scrollable detail, fetched on Enter.
func renderRichDetail(b *model.BookDetail, w int, st styles) string {
	if b == nil || w <= 0 {
		return ""
	}
	var out strings.Builder
	write := func(title, text string) {
		if strings.TrimSpace(text) != "" {
			out.WriteString("\n\n" + st.label.Render(title) + "\n" + st.value.Render(lipgloss.NewStyle().Width(max(1, w)).Render(text)))
		}
	}
	write("About", strings.TrimSpace(b.Headline+"\n"+b.Description))
	write("Editions", strconv.Itoa(b.EditionsCount))
	if ratings := ratingRows(b.RatingsDistribution, w); ratings != "" {
		write("Community ratings", ratings)
	}
	return out.String()
}

// ratingRows renders the array returned by the live API as a half/quarter-star histogram.
func ratingRows(raw string, w int) string {
	buckets, err := model.ParseRatingDistribution(raw)
	if err != nil {
		return "Ratings distribution unavailable"
	}
	maximum := 0
	for _, b := range buckets {
		maximum = max(maximum, b.Count)
	}
	rows := []string{}
	for _, b := range buckets {
		length := 0
		if maximum > 0 {
			length = b.Count * max(1, min(30, w-15)) / maximum
		}
		rows = append(rows, cut(fmt.Sprintf("%4g ★ %s %d", b.Rating, strings.Repeat("█", length), b.Count), w))
	}
	return strings.Join(rows, "\n")
}

type journalModal struct {
	private       bool
	bookID, token int
	title, event  string
	text          textarea.Model
	focusCmd      tea.Cmd
	submitting    bool
	err           string
}

func newJournalModal(sh *shared, st styles, b model.UserBook) *journalModal {
	text := textarea.New()
	text.Placeholder = "Write a note…"
	text.ShowLineNumbers = false
	text.SetStyles(st.textAreaStyles(st.modalBg, st.modalKey, st.modalValue, st.modalDim))
	text.CharLimit = 20000
	text.SetWidth(60)
	text.SetHeight(8)
	return &journalModal{bookID: b.BookID, token: sh.nextToken(), title: b.Book.Title, event: "note", text: text, focusCmd: text.Focus()}
}
func newPrivateNoteModal(sh *shared, st styles, b model.UserBook) *journalModal {
	n := newJournalModal(sh, st, b)
	n.private = true
	n.text.Placeholder = "Write a private note (stored only on this device)…"
	n.text.SetValue(sh.privateNotes[b.BookID])
	return n
}

func (n *journalModal) Update(msg tea.Msg) (bool, tea.Cmd) {
	op := opJournal
	if n.private {
		op = opPrivateNote
	}
	if done, ok := msg.(opDoneMsg); ok && done.op == op && done.seq == n.token {
		n.submitting = false
		if done.err != nil {
			n.err = done.err.Error()
			return false, nil
		}
		return true, nil
	}
	if k, ok := msg.(tea.KeyPressMsg); ok {
		if n.submitting {
			return false, nil
		}
		switch k.String() {
		case "esc":
			return true, nil
		case "tab":
			if n.private {
				return false, nil
			}
			if n.event == "note" {
				n.event = "quote"
				n.text.Placeholder = "Write a quote…"
			} else {
				n.event = "note"
				n.text.Placeholder = "Write a note…"
			}
			return false, nil
		case "ctrl+s":
			if !n.private && strings.TrimSpace(n.text.Value()) == "" {
				n.err = "Enter some text first"
				return false, nil
			}
			n.submitting = true
			n.err = ""
			if n.private {
				return false, request(reqPrivateNote{n.bookID, n.token, n.text.Value()})
			}
			return false, request(reqJournal{n.bookID, n.token, n.event, n.text.Value()})
		}
	}
	var cmd tea.Cmd
	n.text, cmd = n.text.Update(msg)
	return false, cmd
}
func (n *journalModal) View(lay layout, st styles) string {
	width := min(90, max(24, lay.W-4))
	body := st.modalLabel.Render(cut(n.title, modalInnerW(width))) + "\n\n" + n.text.View() + "\n\n"
	if n.submitting {
		body += "Saving…\n"
	}
	if n.err != "" {
		body += st.modalError.Render(lipgloss.NewStyle().Width(modalInnerW(width)).Render(n.err)) + "\n"
	}
	if n.private {
		body += st.modalDim.Render("Only on this device · Ctrl+S save · Esc cancel")
		return renderModalPanel("Private note", body, width, st)
	}
	body += st.modalDim.Render("Tab note/quote · Ctrl+S save · Esc cancel")
	return renderModalPanel("Journal · "+n.event, body, width, st)
}
func (n *journalModal) Keys(k *keyMap) {
	if !n.submitting {
		enable(&k.Back, &k.ReviewSave, &k.ReviewNextField)
		k.ReviewNextField.SetHelp("Tab", "note / quote")
	}
	if n.private {
		k.ReviewNextField.SetEnabled(false)
	}
	k.short = []key.Binding{k.ReviewNextField, k.ReviewSave, k.Back}
}
func (n *journalModal) Resize(lay layout) {
	n.text.SetWidth(modalInnerW(min(90, max(24, lay.W-4))))
	n.text.SetHeight(max(2, min(10, lay.H-12)))
}

func (n *journalModal) Init() tea.Cmd       { return n.focusCmd }
func (n *journalModal) Cursor() *tea.Cursor { return nil }

func renderBookJournal(entries []model.BookJournalEntry, w int, st styles) string {
	if len(entries) == 0 || w <= 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("\n\n" + st.label.Render("Your notes & quotes"))
	for _, entry := range entries {
		out.WriteString("\n\n" + st.dim.Render(cut(entry.ActionAt+" · "+entry.Event, w)) + "\n" + st.value.Render(lipgloss.NewStyle().Width(w).Render(entry.Entry)))
	}
	return out.String()
}
