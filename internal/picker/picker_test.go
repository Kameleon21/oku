package picker

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Kameleon21/oku/internal/model"
)

func newTestPicker(t *testing.T) pickerModel {
	t.Helper()
	books := []model.UserBook{
		{Book: model.Book{ID: 1, Title: "Quicksilver"}},
		{Book: model.Book{ID: 2, Title: "Dune"}},
	}
	items := make([]list.Item, len(books))
	for i, b := range books {
		items[i] = bookItem{book: b}
	}
	l := list.New(items, list.NewDefaultDelegate(), 60, 20)
	l.SetFilteringEnabled(true)
	return pickerModel{list: l}
}

func keyMsg(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func update(t *testing.T, m pickerModel, msg tea.Msg) pickerModel {
	t.Helper()
	upd, _ := m.Update(msg)
	return upd.(pickerModel)
}

func TestTypingQWhileFilteringDoesNotQuit(t *testing.T) {
	m := newTestPicker(t)

	m = update(t, m, keyMsg('/'))
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("FilterState = %v, want Filtering", m.list.FilterState())
	}

	m = update(t, m, keyMsg('q'))
	if m.quitting {
		t.Fatal("typing 'q' inside the filter quit the picker")
	}
	if got := m.list.FilterInput.Value(); got != "q" {
		t.Fatalf("filter input = %q, want %q", got, "q")
	}
}

func TestEscWhileFilteringExitsFilterNotPicker(t *testing.T) {
	m := newTestPicker(t)

	m = update(t, m, keyMsg('/'))
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.quitting {
		t.Fatal("esc inside the filter quit the picker")
	}
	if m.list.FilterState() == list.Filtering {
		t.Fatal("esc did not exit filter mode")
	}
}

func TestQQuitsWhenNotFiltering(t *testing.T) {
	m := newTestPicker(t)

	m = update(t, m, keyMsg('q'))
	if !m.quitting {
		t.Fatal("'q' outside the filter should quit")
	}
	if m.choice != 0 {
		t.Fatalf("choice = %d, want 0 on cancel", m.choice)
	}
}

func TestCtrlCQuitsWhileFiltering(t *testing.T) {
	m := newTestPicker(t)

	m = update(t, m, keyMsg('/'))
	m = update(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})

	if !m.quitting {
		t.Fatal("ctrl+c should quit even while filtering")
	}
}
