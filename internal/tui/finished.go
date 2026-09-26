package tui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/model"
)

type reqStatsBrowse struct{ show bool }

type finishedBookItem struct{ book model.UserBook }

func (i finishedBookItem) Title() string       { return i.book.Book.Title }
func (i finishedBookItem) FilterValue() string { return i.Title() + " " + i.book.Book.AuthorString() }
func (i finishedBookItem) Description() string {
	date := "Unknown date"
	if len(i.book.UserBookReads) > 0 && i.book.UserBookReads[0].FinishedAt != nil {
		date = i.book.UserBookReads[0].FinishedAt.Format("02 Jan 2006")
	}
	rating := "unrated"
	if i.book.Rating > 0 {
		rating = fmt.Sprintf("★ %.1f", i.book.Rating)
	}
	return fmt.Sprintf("%s · %s · %s", date, fallback(i.book.Book.AuthorString(), "Unknown author"), rating)
}
func (s *statsSection) year() int {
	if s.sh.stats != nil {
		return s.sh.stats.Year.Year
	}
	return s.sh.now().Year()
}
func (s *statsSection) rebuildFinished() tea.Cmd {
	selectedID := 0
	if item, ok := s.finished.SelectedItem().(finishedBookItem); ok && len(item.book.UserBookReads) > 0 {
		selectedID = item.book.UserBookReads[0].ID
	}
	var items []list.Item
	selected := s.finished.Index()
	if s.sh.stats != nil {
		for i, b := range s.sh.stats.Finished {
			items = append(items, finishedBookItem{b})
			if selectedID != 0 && len(b.UserBookReads) > 0 && b.UserBookReads[0].ID == selectedID {
				selected = i
			}
		}
	}
	cmd := s.finished.SetItems(items)
	if len(items) > 0 {
		s.finished.Select(min(selected, len(items)-1))
	}
	return cmd
}
func (s *statsSection) finishedKey(msg tea.KeyPressMsg) tea.Cmd {
	k := keysFor(s)
	switch {
	case key.Matches(msg, k.Back, k.FinishedBooks):
		return request(reqStatsBrowse{false})
	case key.Matches(msg, k.ScrollTop):
		s.finished.Select(0)
		return nil
	case key.Matches(msg, k.ScrollBottom):
		if n := len(s.finished.Items()); n > 0 {
			s.finished.Select(n - 1)
		}
		return nil
	case key.Matches(msg, k.HalfPageUp, k.HalfPageDown):
		delta := max(1, s.finished.Paginator.PerPage/2)
		if key.Matches(msg, k.HalfPageUp) {
			delta = -delta
		}
		if n := len(s.finished.Items()); n > 0 {
			s.finished.Select(clampInt(s.finished.Index()+delta, 0, n-1))
		}
		return nil
	case key.Matches(msg, k.Up, k.Down):
		var cmd tea.Cmd
		s.finished, cmd = s.finished.Update(msg)
		return cmd
	case key.Matches(msg, k.Note):
		if b := s.Selected().Book; b != nil {
			return request(reqOpenModal{newPrivateNoteModal(s.sh, s.st, *b)})
		}
	case key.Matches(msg, k.Rate):
		if b := s.Selected().Book; b != nil {
			return request(reqOpenModal{newReviewModal(s.sh, s.st, *b)})
		}
	case key.Matches(msg, k.Refresh):
		return request(reqRefresh{local: true})
	}
	return nil
}
func (s *statsSection) finishedKeys(k *keyMap) {
	enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.Back, &k.FinishedBooks,
		&k.ScrollTop, &k.ScrollBottom, &k.HalfPageUp, &k.HalfPageDown,
		&k.NextSection, &k.PrevSection, &k.TabJump, &k.Sync, &k.Refresh, &k.Search)
	k.Up.SetHelp("k", "navigate")
	k.Down.SetHelp("j", "navigate")
	k.Back.SetHelp("Esc", "back to stats")
	k.FinishedBooks.SetHelp("b", "back to stats")
	k.Note.SetHelp("n", "private note")
	k.Rate.SetHelp("v", "Hardcover review / rate")
	if s.Selected().Book != nil {
		enable(&k.Details, &k.Note, &k.Rate)
	}
	k.short = []key.Binding{k.Help, hint("navigate", k.Down, k.Up), k.Details, k.Note, k.Rate, k.Back, k.FinishedBooks, k.Refresh}
}
