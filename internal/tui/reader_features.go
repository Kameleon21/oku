package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
)

type reqBookDetail struct{ selection selection }
type detailLoadedMsg struct {
	id   int
	book *api.BookDetail
	err  error
}
type journalLoadedMsg struct {
	id      int
	entries []api.JournalEntry
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
		if id == 0 || m.app == nil || m.app.API == nil {
			return m.showToast(toastWarn, "Hardcover is unavailable"), true
		}
		detail := func() tea.Msg { b, err := m.app.GetBookDetail(m.ctx, id, false); return detailLoadedMsg{id, b, err} }
		if r.selection.Book != nil {
			return m.beginLoading(detail, func() tea.Msg {
				entries, err := m.app.API.BookJournal(m.ctx, id)
				return journalLoadedMsg{id, entries, err}
			}), true
		}
		return m.beginLoading(detail), true
	case detailLoadedMsg:
		m.endLoading()
		if r.err != nil {
			return m.showToast(toastError, r.err.Error()), true
		}
		if m.shared.details == nil {
			m.shared.details = map[int]*api.BookDetail{}
		}
		m.shared.details[r.id] = r.book
		m.detail.stamp++
		return nil, true
	case journalLoadedMsg:
		m.endLoading()
		if r.err != nil {
			return m.showToast(toastError, "Load journal: "+r.err.Error()), true
		}
		if m.shared.journals == nil {
			m.shared.journals = map[int][]api.JournalEntry{}
		}
		m.shared.journals[r.id] = r.entries
		m.detail.stamp++
		return nil, true
	case reqPausedShelf:
		if m.app == nil || m.app.API == nil {
			return m.showToast(toastWarn, "Hardcover is unavailable"), true
		}
		return m.beginLoading(func() tea.Msg {
			books, err := m.app.ListBooks(m.ctx, model.StatusPaused, true)
			return pausedLoadedMsg{books, err}
		}), true
	case pausedLoadedMsg:
		m.endLoading()
		if r.err != nil {
			return m.showToast(toastError, r.err.Error()), true
		}
		m.shared.paused = r.books
		return m.broadcast(dataChangedMsg{dataLibrary}), true
	case reqQueueMove:
		if m.isLoading() {
			return m.showToast(toastWarn, inFlightNotice), true
		}
		if m.app == nil || m.app.API == nil {
			return m.showToast(toastWarn, "Hardcover is unavailable"), true
		}
		return m.beginLoading(func() tea.Msg {
			ids, err := m.app.MoveQueueBook(m.ctx, r.bookID, r.delta)
			return queueLoadedMsg{ids, r.bookID, err}
		}), true
	case reqQueueRefresh:
		if m.isLoading() {
			return m.showToast(toastWarn, inFlightNotice), true
		}
		if m.app == nil || m.app.API == nil {
			return m.showToast(toastWarn, "Hardcover is unavailable"), true
		}
		return m.beginLoading(func() tea.Msg { ids, err := m.app.RefreshQueue(m.ctx); return queueLoadedMsg{ids: ids, err: err} }), true
	case queueLoadedMsg:
		m.endLoading()
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
		return m.beginLoading(func() tea.Msg {
			if m.app == nil || m.app.API == nil {
				return searchLoadedMsg{seq: r.seq, err: fmt.Errorf("Hardcover is unavailable")}
			}
			books, err := m.app.TrendingBooks(m.ctx, "week", 20)
			return searchLoadedMsg{results: books, query: "Trending this week", mode: model.SearchModeBook, seq: r.seq, err: err}
		}), true
	case reqJournal:
		if m.isLoading() {
			return m.refuse(opJournal, r.token), true
		}
		if m.app == nil || m.app.API == nil {
			return request(opDoneMsg{op: opJournal, seq: r.token, err: fmt.Errorf("Hardcover is unavailable")}), true
		}
		return m.beginLoading(func() tea.Msg {
			_, err := m.app.AddJournalEntry(m.ctx, r.bookID, r.event, r.text)
			return opDoneMsg{op: opJournal, seq: r.token, err: err, info: "Saved " + r.event}
		}), true
	}
	return nil, false
}

// Rich metadata is appended to the existing scrollable detail, fetched on Enter.
func renderRichDetail(b *api.BookDetail, w int, st styles) string {
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
func ratingRows(raw json.RawMessage, w int) string {
	buckets, err := api.ParseRatingDistribution(raw)
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
func (n *journalModal) Update(msg tea.Msg) (bool, tea.Cmd) {
	if done, ok := msg.(opDoneMsg); ok && done.op == opJournal && done.seq == n.token {
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
			if n.event == "note" {
				n.event = "quote"
				n.text.Placeholder = "Write a quote…"
			} else {
				n.event = "note"
				n.text.Placeholder = "Write a note…"
			}
			return false, nil
		case "ctrl+s":
			if strings.TrimSpace(n.text.Value()) == "" {
				n.err = "Enter some text first"
				return false, nil
			}
			n.submitting = true
			n.err = ""
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
	body += st.modalDim.Render("Tab note/quote · Ctrl+S save · Esc cancel")
	return renderModalPanel("Journal · "+n.event, body, width, st)
}
func (n *journalModal) Keys(k *keyMap) {
	if !n.submitting {
		enable(&k.Back, &k.ReviewSave, &k.ReviewNextField)
		k.ReviewNextField.SetHelp("Tab", "note / quote")
	}
	k.short = []key.Binding{k.ReviewNextField, k.ReviewSave, k.Back}
}
func (n *journalModal) Resize(lay layout) {
	n.text.SetWidth(modalInnerW(min(90, max(24, lay.W-4))))
	n.text.SetHeight(max(2, min(10, lay.H-12)))
}

func (n *journalModal) Init() tea.Cmd       { return n.focusCmd }
func (n *journalModal) Cursor() *tea.Cursor { return nil }

func renderBookJournal(entries []api.JournalEntry, w int, st styles) string {
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
