package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// newTestDashboard builds a dashboard with no app, so every command that would
// touch the network or the store reports an error instead.
func newTestDashboard() dashboardModel {
	return newDashboardModel(context.Background(), nil)
}

func TestSearchInputNormalModeNavigation(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionSearch
	m.searchSub = searchSubInput
	m.searchMode = searchModeNormal
	m.searchInput.SetValue("dune")

	// Press 'k' (up) → should go to previous section (sectionOku).
	updated, _ := m.updateLibraryMode(runeKey('k'))
	got := updated.(dashboardModel)

	if got.section != sectionOku {
		t.Fatalf("section after k = %v, want %v", got.section, sectionOku)
	}
	if got.searchInput.Value() != "dune" {
		t.Fatalf("search input changed in normal mode, got %q", got.searchInput.Value())
	}
	if got.searchMode != searchModeNormal {
		t.Fatalf("search mode = %v, want normal", got.searchMode)
	}
}

func TestLoadLibraryCmdWithNilAppReturnsError(t *testing.T) {
	cmd := loadLibraryCmd(context.Background(), nil, false)
	if cmd == nil {
		t.Fatal("loadLibraryCmd() returned nil cmd")
	}

	msg := cmd()
	loaded, ok := msg.(libraryLoadedMsg)
	if !ok {
		t.Fatalf("loadLibraryCmd() msg type = %T, want libraryLoadedMsg", msg)
	}
	if loaded.err == nil {
		t.Fatal("expected libraryLoadedMsg error for nil app")
	}
}

func TestLoadCachedLibraryCmdWithNilAppReturnsError(t *testing.T) {
	cmd := loadCachedLibraryCmd(nil)
	if cmd == nil {
		t.Fatal("loadCachedLibraryCmd() returned nil cmd")
	}

	msg := cmd()
	loaded, ok := msg.(libraryLoadedMsg)
	if !ok {
		t.Fatalf("loadCachedLibraryCmd() msg type = %T, want libraryLoadedMsg", msg)
	}
	if loaded.err == nil {
		t.Fatal("expected libraryLoadedMsg error for nil app")
	}
}

func TestStaleCachedLibraryRendersBeforeRefresh(t *testing.T) {
	m := newTestDashboard()
	updated, cmd := m.updateLibraryMode(libraryLoadedMsg{
		reading:      []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}},
		needsRefresh: true,
	})
	got := updated.(dashboardModel)

	if !got.loaded {
		t.Fatal("cached library should mark dashboard loaded")
	}
	if !got.loading {
		t.Fatal("stale cache should start a background refresh")
	}
	if len(got.readingBooks) != 1 || got.readingBooks[0].Book.Title != "Dune" {
		t.Fatalf("cached reading books = %#v, want Dune", got.readingBooks)
	}
	if cmd == nil {
		t.Fatal("stale cache should return a refresh command")
	}
}

func TestSearchInputInsertModeTypingAndEsc(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionSearch
	m.searchSub = searchSubInput
	m.enterSearchInsertMode()

	updated, _ := m.updateLibraryMode(runeKey('h'))
	got := updated.(dashboardModel)

	if got.section != sectionSearch {
		t.Fatalf("section after typing in insert mode = %v, want %v", got.section, sectionSearch)
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
	if got.section != sectionSearch {
		t.Fatalf("section after esc = %v, want %v", got.section, sectionSearch)
	}
}

func TestLibrarySectionVimNavigation(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionReading

	updated, _ := m.updateLibraryMode(runeKey('l'))
	got := updated.(dashboardModel)
	if got.section != sectionOku {
		t.Fatalf("section after l = %v, want %v", got.section, sectionOku)
	}

	updated, _ = got.updateLibraryMode(runeKey('h'))
	got = updated.(dashboardModel)
	if got.section != sectionReading {
		t.Fatalf("section after h = %v, want %v", got.section, sectionReading)
	}
}

func TestSearchInputNormalModeEscGoesBack(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionSearch
	m.searchSub = searchSubInput
	m.searchMode = searchModeNormal

	updated, _ := m.updateLibraryMode(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(dashboardModel)
	if got.section != sectionOku {
		t.Fatalf("section after Esc = %v, want %v", got.section, sectionOku)
	}
}

func TestSubmitSearchSetsLoadingState(t *testing.T) {
	m := newTestDashboard()
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
	m := newTestDashboard()
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
	m := newTestDashboard()
	m.loading = true
	m.searchLoading = true
	m.section = sectionSearch
	m.searchSub = searchSubInput

	updated, _ := m.updateLibraryMode(searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}},
		query:   "dune",
		mode:    model.SearchModeAuthor,
	})
	got := updated.(dashboardModel)

	if got.loading || got.searchLoading {
		t.Fatalf("loading flags after response = loading:%v searchLoading:%v, want false/false", got.loading, got.searchLoading)
	}
	if got.searchSub != searchSubResults {
		t.Fatalf("searchSub = %v, want %v", got.searchSub, searchSubResults)
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

func TestSlashSearchPreservesExistingQuery(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionReading
	m.searchInput.SetValue("dune")

	updated, _ := m.updateLibraryMode(runeKey('/'))
	got := updated.(dashboardModel)

	if got.section != sectionSearch {
		t.Fatalf("section after / = %v, want %v", got.section, sectionSearch)
	}
	if got.searchSub != searchSubInput {
		t.Fatalf("searchSub after / = %v, want %v", got.searchSub, searchSubInput)
	}
	if got.searchInput.Value() != "dune" {
		t.Fatalf("search query after / = %q, want %q", got.searchInput.Value(), "dune")
	}
}

func TestTimerStartOpensBookSelectionFirst(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionTimer
	m.readingBooks = []model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune"}},
		{Book: model.Book{ID: 2, Title: "Foundation"}},
	}

	updated, cmd := m.updateLibraryMode(runeKey('t'))
	got := updated.(dashboardModel)

	if cmd != nil {
		t.Fatal("timer start should open selection first, got immediate command")
	}
	if !got.timerSelecting {
		t.Fatal("timerSelecting = false, want true after pressing t")
	}
}

func TestSearchResultsKStaysInResults(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionSearch
	m.searchSub = searchSubResults
	m.searchMode = searchModeNormal
	m.searchList.SetItems([]list.Item{
		searchResultItem{result: model.SearchResult{ID: 1, Title: "Dune"}},
		searchResultItem{result: model.SearchResult{ID: 2, Title: "Dune Messiah"}},
	})
	m.searchList.Select(1)

	updated, _ := m.updateLibraryMode(runeKey('k'))
	got := updated.(dashboardModel)

	if got.searchSub != searchSubResults {
		t.Fatalf("searchSub after k = %v, want %v", got.searchSub, searchSubResults)
	}
	if got.section != sectionSearch {
		t.Fatalf("section after k = %v, want %v", got.section, sectionSearch)
	}
}

func TestCycleDensityRefreshesSearchResultItems(t *testing.T) {
	m := newTestDashboard()
	m.density = densityDefault
	m.searchBooks = []model.SearchResult{
		{ID: 1, Title: "Dune", Authors: []string{"Frank Herbert"}, Slug: "dune"},
	}
	m.refreshSearchResultItems()

	m.cycleDensity()

	items := m.searchList.Items()
	if len(items) != 1 {
		t.Fatalf("search items len = %d, want 1", len(items))
	}
	item, ok := items[0].(searchResultItem)
	if !ok {
		t.Fatalf("item type = %T, want searchResultItem", items[0])
	}
	if item.density != densityVerbose {
		t.Fatalf("search item density = %v, want %v", item.density, densityVerbose)
	}
	if !strings.Contains(item.Description(), "slug:") {
		t.Fatalf("verbose search description = %q, expected slug details", item.Description())
	}
}

func TestParseReviewRatingInput(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    float64
		wantErr bool
	}{
		{name: "empty is unrated", raw: "", want: 0},
		{name: "whole", raw: "4", want: 4},
		{name: "half", raw: "4.5", want: 4.5},
		{name: "invalid increment", raw: "4.3", wantErr: true},
		{name: "not numeric", raw: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseReviewRatingInput(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseReviewRatingInput(%q) expected error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseReviewRatingInput(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("parseReviewRatingInput(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestReviewSaveClosesModalAndShowsPendingState(t *testing.T) {
	m := newTestDashboard()
	m.loaded = true
	m.openReviewRatingModal(model.UserBook{
		Book: model.Book{ID: 42, Title: "Dune"},
	})
	m.reviewRatingInput.SetValue("3")
	m.reviewTextInput.SetValue("Strong first half.")

	updated, cmd := m.updateReviewRatingMode(tea.KeyMsg{Type: tea.KeyCtrlS})
	got := updated.(dashboardModel)

	if cmd == nil {
		t.Fatal("expected save command")
	}
	if got.mode != modeLibrary {
		t.Fatalf("mode after save = %v, want library", got.mode)
	}
	if got.reviewBook != nil {
		t.Fatal("review modal should be closed after save starts")
	}
	if !got.loading {
		t.Fatal("loading should be true while save is in flight")
	}
	if got.infoMsg != "Saving review..." {
		t.Fatalf("infoMsg = %q, want Saving review...", got.infoMsg)
	}
}

func TestTimerSelectEnterClampsStaleIndex(t *testing.T) {
	m := newTestDashboard()
	m.timerSelecting = true
	m.timerSelectIdx = 5 // stale: list shrank while the picker was open
	m.readingBooks = []model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune"}},
	}

	updated, cmd := m.handleTimerKeys(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(dashboardModel)

	if cmd == nil {
		t.Fatal("enter on a valid list should start the timer")
	}
	if got.timerSelecting {
		t.Fatal("timerSelecting should be false after enter")
	}
}

func TestShouldUseDemoLocalDataIsOptInOnly(t *testing.T) {
	t.Setenv("OKU_TUI_DEMO_DATA", "")
	if shouldUseDemoLocalData() {
		t.Fatal("demo data must not show without OKU_TUI_DEMO_DATA=1")
	}

	t.Setenv("OKU_TUI_DEMO_DATA", "1")
	if !shouldUseDemoLocalData() {
		t.Fatal("OKU_TUI_DEMO_DATA=1 should enable demo data")
	}
}
