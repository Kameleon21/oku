package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/model"
)

func TestTrendingFromDefaultSearchInput(t *testing.T) {
	for _, draft := range []string{"", "Dune"} {
		t.Run("draft="+draft, func(t *testing.T) {
			m := newGoldenModel(t, 80, 24, tabReading)
			send(t, m, runeKey('3')) // The default entry path focuses the empty text box.
			s := searchOf(m)
			if s.focus != inputFocused {
				t.Fatal("expected default input focus")
			}
			s.input.SetValue(draft)
			if !strings.Contains(stripANSI(m.frame()), "C-d trending") {
				t.Fatal("input does not advertise discovery")
			}
			cmd := send(t, m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
			if cmd == nil || s.focus != resultsFocused || !s.loading || s.seq != 1 {
				t.Fatalf("discovery not dispatched: focus=%v loading=%v seq=%d", s.focus, s.loading, s.seq)
			}
			if s.input.Value() != draft {
				t.Fatal("discovery changed draft")
			}
			send(t, m, searchLoadedMsg{seq: s.seq, query: "Trending this week", mode: model.SearchModeBook, results: []model.SearchResult{{ID: 123, Title: "Trending book"}}})
			if s.loading || s.selected() == nil || s.selected().ID != 123 || s.focus != resultsFocused {
				t.Fatal("trending results not navigable")
			}
			send(t, m, runeKey('i'))
			if s.focus != inputFocused || s.input.Value() != draft {
				t.Fatal("could not resume draft")
			}
		})
	}
}

func TestCapitalDStillTypesInSearch(t *testing.T) {
	for _, msg := range []tea.KeyPressMsg{runeKey('D'), {Code: 'd', Mod: tea.ModShift, Text: "D"}} {
		m := newGoldenModel(t, 80, 24, tabSearch)
		s := searchOf(m)
		s.focusInput()
		send(t, m, msg)
		if s.input.Value() != "D" || s.loading || s.seq != 0 || s.focus != inputFocused {
			t.Fatal("capital D was stolen from query input")
		}
	}
}

func TestTrendingFailureClearsLoadingAndKeepsPreviousResults(t *testing.T) {
	m := newGoldenModel(t, 80, 24, tabSearch)
	s := searchOf(m)
	s.results = []model.SearchResult{{ID: 8, Title: "Previous result"}}
	s.lastQuery = "previous"
	s.rebuildResults()
	s.focusInput()
	s.input.SetValue("new draft")
	req := s.handleKey(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})().(reqTrending)
	cmd, _ := m.handleReaderRequest(req) // No API configured: exercise the real error command.
	var deliver func(tea.Cmd) bool
	deliver = func(cmd tea.Cmd) bool {
		if cmd == nil {
			return false
		}
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, child := range msg {
				if deliver(child) {
					return true
				}
			}
		case searchLoadedMsg:
			if msg.err == nil {
				t.Fatal("expected missing API error")
			}
			send(t, m, msg)
			return true
		}
		return false
	}
	if !deliver(cmd) {
		t.Fatal("error did not produce a search result message")
	}
	if s.loading || m.isLoading() || s.input.Value() != "new draft" || s.lastQuery != "previous" || len(s.results) != 1 || s.results[0].ID != 8 {
		t.Fatal("failure left spinner running or erased previous state")
	}
}

func TestTrendingRepeatDropsOlderResponse(t *testing.T) {
	m := newGoldenModel(t, 80, 24, tabSearch)
	s := searchOf(m)
	s.focusInput()
	first := s.handleKey(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})().(reqTrending)
	second := s.handleKey(runeKey('D'))().(reqTrending)
	s.applyLoaded(searchLoadedMsg{seq: second.seq, results: []model.SearchResult{{ID: 2, Title: "Current"}}})
	s.applyLoaded(searchLoadedMsg{seq: first.seq, results: []model.SearchResult{{ID: 1, Title: "Old"}}})
	if s.loading || len(s.results) != 1 || s.results[0].ID != 2 {
		t.Fatal("old discovery response replaced the latest results")
	}
}
