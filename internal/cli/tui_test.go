package cli

import (
	"strings"
	"testing"

	"github.com/Kameleon21/oku/internal/model"
	tea "github.com/charmbracelet/bubbletea"
)

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestSearchInputNormalModeUsesVimPaneNavigation(t *testing.T) {
	m := newDashboardModel(nil)
	m.focus = focusSearchInput
	m.searchMode = searchModeNormal
	m.searchInput.SetValue("dune")

	updated, _ := m.updateLibraryMode(runeKey('h'))
	got := updated.(dashboardModel)

	if got.focus != focusOku {
		t.Fatalf("focus after h = %v, want %v", got.focus, focusOku)
	}
	if got.searchInput.Value() != "dune" {
		t.Fatalf("search input changed in normal mode, got %q", got.searchInput.Value())
	}
	if got.searchMode != searchModeNormal {
		t.Fatalf("search mode = %v, want normal", got.searchMode)
	}
}

func TestSearchInputInsertModeTypingAndEsc(t *testing.T) {
	m := newDashboardModel(nil)
	m.focus = focusSearchInput
	m.enterSearchInsertMode()

	updated, _ := m.updateLibraryMode(runeKey('h'))
	got := updated.(dashboardModel)

	if got.focus != focusSearchInput {
		t.Fatalf("focus after typing in insert mode = %v, want %v", got.focus, focusSearchInput)
	}
	if got.searchInput.Value() != "h" {
		t.Fatalf("search input value = %q, want %q", got.searchInput.Value(), "h")
	}
	if got.searchMode != searchModeInsert {
		t.Fatalf("search mode after typing = %v, want insert", got.searchMode)
	}

	updated, _ = got.updateLibraryMode(tea.KeyMsg{Type: tea.KeyEsc})
	got = updated.(dashboardModel)
	if got.searchMode != searchModeNormal {
		t.Fatalf("search mode after esc = %v, want normal", got.searchMode)
	}
	if got.focus != focusSearchInput {
		t.Fatalf("focus after esc = %v, want %v", got.focus, focusSearchInput)
	}
}

func TestSubmitSearchSetsLoadingState(t *testing.T) {
	m := newDashboardModel(nil)
	m.searchInput.SetValue("dune")
	m.searchQueryMode = model.SearchModeAuthor

	cmd := m.submitSearch()
	if cmd == nil {
		t.Fatal("submitSearch() returned nil cmd for non-empty query")
	}
	if !m.loading || !m.searchLoading {
		t.Fatalf("loading flags = loading:%v searchLoading:%v, want true/true", m.loading, m.searchLoading)
	}
	if m.searchLoadingQuery != "dune" {
		t.Fatalf("searchLoadingQuery = %q, want dune", m.searchLoadingQuery)
	}
	if !strings.Contains(m.searchList.Title, "loading") {
		t.Fatalf("search list title %q does not include loading state", m.searchList.Title)
	}
	if !strings.Contains(strings.ToLower(m.infoMsg), "searching for") {
		t.Fatalf("info message %q does not include searching feedback", m.infoMsg)
	}
}

func TestSubmitSearchGuardAndEmptyValidation(t *testing.T) {
	m := newDashboardModel(nil)
	m.searchLoading = true
	m.searchInput.SetValue("dune")
	if cmd := m.submitSearch(); cmd != nil {
		t.Fatal("submitSearch() should return nil while search is already loading")
	}

	m.searchLoading = false
	m.searchInput.SetValue("   ")
	if cmd := m.submitSearch(); cmd != nil {
		t.Fatal("submitSearch() should return nil for empty query")
	}
	if m.errMsg == "" {
		t.Fatal("expected validation error for empty query")
	}
}

func TestSearchLoadedMsgTransitionsToResults(t *testing.T) {
	m := newDashboardModel(nil)
	m.loading = true
	m.searchLoading = true
	m.focus = focusSearchInput

	updated, _ := m.updateLibraryMode(searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}},
		query:   "dune",
		mode:    model.SearchModeAuthor,
	})
	got := updated.(dashboardModel)

	if got.loading || got.searchLoading {
		t.Fatalf("loading flags after response = loading:%v searchLoading:%v, want false/false", got.loading, got.searchLoading)
	}
	if got.focus != focusSearchResults {
		t.Fatalf("focus = %v, want %v", got.focus, focusSearchResults)
	}
	if got.lastQuery != "dune" {
		t.Fatalf("lastQuery = %q, want dune", got.lastQuery)
	}
	if got.searchQueryMode != model.SearchModeAuthor {
		t.Fatalf("searchQueryMode = %q, want %q", got.searchQueryMode, model.SearchModeAuthor)
	}
	if got.searchList.Title != "AUTHOR Results (1)" {
		t.Fatalf("searchList title = %q, want %q", got.searchList.Title, "AUTHOR Results (1)")
	}
	if !strings.Contains(got.infoMsg, "loaded 1 results") {
		t.Fatalf("info message = %q, expected loaded-count feedback", got.infoMsg)
	}
}
