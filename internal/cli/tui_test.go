package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// newTestDashboard builds a dashboard with no app, so every command that would
// touch the network or the store reports an error instead.
func newTestDashboard() dashboardModel {
	m := newDashboardModel(context.Background(), nil)
	// Tests drive Update directly and never run Init, so nothing is in flight.
	m.inflight = 0
	return m
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
	updated, cmd := m.Update(libraryLoadedMsg{
		reading:      []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}},
		needsRefresh: true,
	})
	got := updated.(dashboardModel)

	if !got.loaded {
		t.Fatal("cached library should mark dashboard loaded")
	}
	if !got.isLoading() {
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
	if !m.isLoading() || !m.searchLoading {
		t.Fatalf("loading flags = loading:%v searchLoading:%v, want true/true", m.isLoading(), m.searchLoading)
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
	if cmd := m.submitSearch(); cmd == nil {
		t.Fatal("a query typed over an in-flight search should be searchable")
	}

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
	m.inflight = 1
	m.searchLoading = true
	m.section = sectionSearch
	m.searchSub = searchSubInput
	m.searchQueryMode = model.SearchModeAuthor

	updated, _ := m.Update(searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}},
		query:   "dune",
		mode:    model.SearchModeAuthor,
	})
	got := updated.(dashboardModel)

	if got.isLoading() || got.searchLoading {
		t.Fatalf("loading flags after response = loading:%v searchLoading:%v, want false/false", got.isLoading(), got.searchLoading)
	}

	if got.searchSub != searchSubResults {
		t.Fatalf("searchSub = %v, want %v", got.searchSub, searchSubResults)
	}
	if got.lastQuery != "dune" {
		t.Fatalf("lastQuery = %q, want dune", got.lastQuery)
	}
	if got.lastSearchMode != model.SearchModeAuthor {
		t.Fatalf("lastSearchMode = %q, want %q", got.lastSearchMode, model.SearchModeAuthor)
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

func TestReviewSaveKeepsModalOpenWhileSaving(t *testing.T) {
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
	if got.mode != modeReviewRating {
		t.Fatalf("mode while saving = %v, want review/rating", got.mode)
	}
	if !got.reviewSubmitting {
		t.Fatal("reviewSubmitting should be true while the save is in flight")
	}
	if got.reviewBook == nil {
		t.Fatal("review modal should stay open until the save succeeds")
	}
	if !got.isLoading() {
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
func TestAsyncResultsAreAppliedInEveryMode(t *testing.T) {
	tests := []struct {
		name string
		mode viewMode
	}{
		{name: "library", mode: modeLibrary},
		{name: "page modal", mode: modeUpdatePage},
		{name: "review modal", mode: modeReviewRating},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestDashboard()
			m.loaded = true
			m.section = sectionSearch
			m.searchSub = searchSubInput
			m.enterSearchInsertMode()
			m.searchInput.SetValue("dune")
			if cmd := m.submitSearch(); cmd == nil {
				t.Fatal("submitSearch() returned nil cmd")
			}

			m.mode = tt.mode
			m.pageInput.SetValue("120")
			m.openReviewRatingModal(model.UserBook{Book: model.Book{ID: 7, Title: "Dune"}})
			m.mode = tt.mode

			updated, _ := m.Update(searchLoadedMsg{
				results: []model.SearchResult{{ID: 1, Title: "Dune"}},
				query:   "dune",
				mode:    model.SearchModeBook,
				seq:     m.searchSeq,
			})
			got := updated.(dashboardModel)

			if got.searchLoading {
				t.Fatal("searchLoading should be cleared by the result, whatever the mode")
			}
			if got.isLoading() {
				t.Fatal("loading should be cleared by the result, whatever the mode")
			}
			if len(got.searchBooks) != 1 {
				t.Fatalf("searchBooks = %d, want 1", len(got.searchBooks))
			}
			if got.mode != tt.mode {
				t.Fatalf("mode = %v, want %v: an async result must not close a modal", got.mode, tt.mode)
			}
			if got.searchSub != searchSubResults {
				t.Fatalf("searchSub = %v, want %v", got.searchSub, searchSubResults)
			}
			if got.searchMode != searchModeNormal {
				t.Fatal("results should leave search insert mode so j/k scroll them")
			}
			if tt.mode == modeUpdatePage && got.pageInput.Value() != "120" {
				t.Fatalf("page input = %q, want it untouched", got.pageInput.Value())
			}
		})
	}
}

func TestStaleSearchResultIsIgnored(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionSearch
	m.searchInput.SetValue("dune")
	if cmd := m.submitSearch(); cmd == nil {
		t.Fatal("submitSearch() returned nil cmd for the first query")
	}
	staleSeq := m.searchSeq

	// The user retypes before the first response lands.
	m.searchLoading = false
	m.searchInput.SetValue("foundation")
	if cmd := m.submitSearch(); cmd == nil {
		t.Fatal("submitSearch() returned nil cmd for the second query")
	}

	updated, cmd := m.Update(searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}},
		query:   "dune",
		mode:    model.SearchModeBook,
		seq:     staleSeq,
	})
	got := updated.(dashboardModel)

	if cmd != nil {
		t.Fatal("a superseded search result should produce no command")
	}
	if !got.searchLoading {
		t.Fatal("a superseded result must not clear the in-flight search")
	}
	if len(got.searchBooks) != 0 {
		t.Fatalf("searchBooks = %d, want 0 for a superseded result", len(got.searchBooks))
	}

	updated, _ = got.Update(searchLoadedMsg{
		results: []model.SearchResult{{ID: 2, Title: "Foundation"}},
		query:   "foundation",
		mode:    model.SearchModeBook,
		seq:     got.searchSeq,
	})
	got = updated.(dashboardModel)

	if got.searchLoading {
		t.Fatal("the latest result should clear searchLoading")
	}
	if got.lastQuery != "foundation" {
		t.Fatalf("lastQuery = %q, want foundation", got.lastQuery)
	}
}

func TestSearchResultKeepsUserSelectedMode(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionSearch
	m.searchQueryMode = model.SearchModeGenre

	updated, _ := m.Update(searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}},
		query:   "dune",
		mode:    model.SearchModeBook,
		seq:     m.searchSeq,
	})
	got := updated.(dashboardModel)

	if got.searchQueryMode != model.SearchModeGenre {
		t.Fatalf("searchQueryMode = %q, want the mode the user picked (%q)", got.searchQueryMode, model.SearchModeGenre)
	}
	if got.searchList.Title != "BOOK Results (1)" {
		t.Fatalf("searchList title = %q, want the mode the results came from", got.searchList.Title)
	}
}

func TestTimerOpDoneReleasesItsSlot(t *testing.T) {
	m := newTestDashboard()
	m.inflight = 1
	m.timerSelecting = true

	updated, cmd := m.Update(timerOpDoneMsg{info: "Timer started — Dune"})
	got := updated.(dashboardModel)

	// The timer operation's slot is released; only the local-data reload it
	// starts is left in flight.
	if got.inflight != 1 {
		t.Fatalf("inflight = %d, want 1 (the reload): the timer op must release its slot", got.inflight)
	}
	if got.timerSelecting {
		t.Fatal("timerOpDoneMsg should close the timer picker")
	}
	if got.infoMsg != "Timer started — Dune" {
		t.Fatalf("infoMsg = %q, want the timer info", got.infoMsg)
	}
	if cmd == nil {
		t.Fatal("timer operations should reload local data")
	}

	updated, _ = got.Update(localDataLoadedMsg{})
	if updated.(dashboardModel).isLoading() {
		t.Fatal("loading should clear once the reload lands")
	}
}

func TestLocalDataClearsLoadingAndArmsTimerTick(t *testing.T) {
	m := newTestDashboard()
	m.inflight = 1
	m.timerTicking = true

	// With no timer running the one-second tick loop stops.
	updated, cmd := m.Update(timerTickMsg(time.Now()))
	got := updated.(dashboardModel)
	if cmd != nil {
		t.Fatal("the timer tick should not re-arm while no timer runs")
	}
	if got.timerTicking {
		t.Fatal("timerTicking should be cleared when no timer runs")
	}

	updated, cmd = got.Update(localDataLoadedMsg{
		timerState: &model.TimerState{BookID: 1, StartedAt: time.Now()},
		timerBook:  &model.Book{ID: 1, Title: "Dune"},
	})
	got = updated.(dashboardModel)

	if got.isLoading() {
		t.Fatal("localDataLoadedMsg should clear loading")
	}
	if cmd == nil || !got.timerTicking {
		t.Fatal("a running timer should arm the one-second tick")
	}
	if got.timerBook == nil || got.timerBook.Title != "Dune" {
		t.Fatal("the running timer's book should be resolved into model state")
	}
}

func TestSpinnerStopsWhenNothingIsInFlight(t *testing.T) {
	m := newTestDashboard()
	m.spinning = true
	m.searchLoading = false

	updated, cmd := m.Update(spinner.TickMsg{})
	got := updated.(dashboardModel)

	if cmd != nil {
		t.Fatal("an idle dashboard should stop re-arming the spinner")
	}
	if got.spinning {
		t.Fatal("spinning should be cleared once nothing is in flight")
	}
	if got.beginLoading(loadLocalDataCmd(got.app)) == nil {
		t.Fatal("starting new work should re-arm the spinner")
	}
	if cmd := got.beginLoading(loadLocalDataCmd(got.app)); cmd == nil {
		t.Fatal("the second operation should still be batched")
	} else if !got.spinning || got.inflight != 2 {
		t.Fatalf("inflight = %d, spinning = %v: a second operation must be counted without a second tick loop", got.inflight, got.spinning)
	}
}

func TestOverlappingLoadsKeepLoadingUntilBothFinish(t *testing.T) {
	m := newTestDashboard()
	if cmd := m.beginLoading(loadLocalDataCmd(m.app), loadLibraryCmd(m.ctx, m.app, false)); cmd == nil {
		t.Fatal("beginLoading() returned no command")
	}
	if !m.isLoading() {
		t.Fatal("two commands are in flight")
	}

	updated, _ := m.Update(localDataLoadedMsg{})
	got := updated.(dashboardModel)
	if !got.isLoading() {
		t.Fatal("the first result must not clear loading while the second load runs")
	}

	updated, _ = got.Update(libraryLoadedMsg{})
	got = updated.(dashboardModel)
	if got.isLoading() {
		t.Fatal("loading should clear once every command has reported")
	}
}

func TestQuickProgressIsGuardedWhileInFlight(t *testing.T) {
	m := newTestDashboard()
	m.loaded = true
	m.inflight = 0
	m.section = sectionReading
	m.readingBooks = []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}}
	m.refreshListItems()

	updated, cmd := m.Update(runeKey('+'))
	got := updated.(dashboardModel)
	if cmd == nil {
		t.Fatal("the first + should start a progress update")
	}
	if !got.isLoading() {
		t.Fatal("the first + should mark the update in flight")
	}

	updated, cmd = got.Update(runeKey('+'))
	got = updated.(dashboardModel)
	if cmd != nil {
		t.Fatal("a second + while an update is in flight must not fire: it would lose one")
	}
	if got.infoMsg == "" {
		t.Fatal("the refused update should say why")
	}
}

func TestReviewSaveFailureKeepsModalOpen(t *testing.T) {
	m := newTestDashboard()
	m.loaded = true
	m.openReviewRatingModal(model.UserBook{Book: model.Book{ID: 42, Title: "Dune"}})
	m.reviewRatingInput.SetValue("3")
	m.reviewTextInput.SetValue("Strong first half.")

	updated, _ := m.updateReviewRatingMode(tea.KeyMsg{Type: tea.KeyCtrlS})
	got := updated.(dashboardModel)

	updated, cmd := got.Update(opDoneMsg{op: opReview, seq: got.reviewSeq, err: errors.New("save failed")})

	got = updated.(dashboardModel)

	if cmd != nil {
		t.Fatal("a failed save should not trigger a reload")
	}
	if got.mode != modeReviewRating || got.reviewBook == nil {
		t.Fatal("a failed save must keep the modal open")
	}
	if got.reviewSubmitting {
		t.Fatal("reviewSubmitting should be cleared once the save failed")
	}
	if got.isLoading() {
		t.Fatal("loading should be cleared once the save failed")
	}
	if got.reviewTextInput.Value() != "Strong first half." {
		t.Fatalf("review text = %q, want the draft to survive", got.reviewTextInput.Value())
	}
	if got.reviewErr != "save failed" {
		t.Fatalf("reviewErr = %q, want save failed", got.reviewErr)
	}

	if !strings.Contains(got.reviewRatingOverlay(), "save failed") {
		t.Fatal("the failure should be visible inside the overlay, not behind it")
	}
}

func TestUnrelatedOpDoesNotDisturbReviewModal(t *testing.T) {
	m := newTestDashboard()
	m.loaded = true
	m.openReviewRatingModal(model.UserBook{Book: model.Book{ID: 42, Title: "Dune"}})
	m.reviewTextInput.SetValue("half written")

	updated, _ := m.Update(opDoneMsg{
		op:        opProgress,
		info:      "Progress +10 → page 40",
		reload:    true,
		markDirty: true,
	})
	got := updated.(dashboardModel)

	if got.mode != modeReviewRating || got.reviewBook == nil {
		t.Fatal("another operation's result must not close the review modal")
	}
	if got.reviewTextInput.Value() != "half written" {
		t.Fatalf("review text = %q, want the draft untouched", got.reviewTextInput.Value())
	}
	if !got.dirty {
		t.Fatal("the unrelated mutation should still mark the library dirty")
	}
	if got.infoMsg != "Progress +10 → page 40" {
		t.Fatalf("infoMsg = %q, want the unrelated result reported", got.infoMsg)
	}
}

func TestReviewSaveSuccessClosesModal(t *testing.T) {
	m := newTestDashboard()
	m.loaded = true
	m.openReviewRatingModal(model.UserBook{Book: model.Book{ID: 42, Title: "Dune"}})
	m.reviewRatingInput.SetValue("3")

	updated, _ := m.updateReviewRatingMode(tea.KeyMsg{Type: tea.KeyCtrlS})
	got := updated.(dashboardModel)

	updated, cmd := got.Update(opDoneMsg{
		op:        opReview,
		seq:       got.reviewSeq,
		info:      "Updated rating (★★★)",
		reload:    true,
		markDirty: true,
	})

	got = updated.(dashboardModel)

	if got.mode != modeLibrary || got.reviewBook != nil {
		t.Fatal("a successful save should close the modal")
	}
	if cmd == nil {
		t.Fatal("a successful save should reload the library")
	}
	if !got.dirty {
		t.Fatal("a successful save should mark the library dirty")
	}
}

func TestBackgroundReconcileClearsDirtyOnlyOnSuccess(t *testing.T) {
	m := newTestDashboard()
	m.loaded = true
	m.inflight = 0
	m.dirty = true
	m.lastMutationAt = time.Now().Add(-2 * backgroundSyncWindow)

	updated, cmd := m.Update(backgroundCheckMsg{})
	started := updated.(dashboardModel)
	if cmd == nil {
		t.Fatal("an overdue reconcile should start a refresh")
	}
	if !started.dirty {
		t.Fatal("dirty must stay set until the reconcile actually succeeds")
	}
	if !started.reconciling {
		t.Fatal("reconciling should mark the in-flight reconcile")
	}

	updated, _ = started.Update(libraryLoadedMsg{reconcile: true, err: errors.New("offline")})
	failed := updated.(dashboardModel)
	if !failed.dirty {
		t.Fatal("a failed reconcile must leave the library dirty")
	}
	if failed.reconciling {
		t.Fatal("a failed reconcile should release the reconcile slot so it can retry")
	}

	updated, _ = started.Update(libraryLoadedMsg{reconcile: true})
	succeeded := updated.(dashboardModel)
	if succeeded.dirty {
		t.Fatal("a successful reconcile should clear dirty")
	}
}

func TestUnrelatedLibraryLoadDoesNotClearDirty(t *testing.T) {
	m := newTestDashboard()
	m.loaded = true
	m.dirty = true
	m.lastMutationAt = time.Now().Add(-2 * backgroundSyncWindow)

	updated, _ := m.Update(backgroundCheckMsg{})
	got := updated.(dashboardModel)
	if !got.reconciling {
		t.Fatal("the reconcile should be in flight")
	}

	// A status change lands while the reconcile is still running and triggers
	// its own cache reload, whose result arrives first.
	updated, _ = got.Update(opDoneMsg{op: opStatus, info: "Status changed", reload: true, markDirty: true})
	got = updated.(dashboardModel)
	updated, _ = got.Update(libraryLoadedMsg{})
	got = updated.(dashboardModel)

	if !got.dirty {
		t.Fatal("an unrelated library load must not clear dirty")
	}
	if !got.reconciling {
		t.Fatal("the reconcile is still in flight")
	}

	updated, _ = got.Update(libraryLoadedMsg{reconcile: true})
	got = updated.(dashboardModel)
	if got.dirty || got.reconciling {
		t.Fatalf("the reconcile result should clear dirty: dirty=%v reconciling=%v", got.dirty, got.reconciling)
	}
}

func TestPendingProgressResultDoesNotClosePageModal(t *testing.T) {
	m := newTestDashboard()
	m.loaded = true
	m.section = sectionReading
	m.readingBooks = []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}}
	m.refreshListItems()

	updated, _ := m.Update(runeKey('+')) // progress update in flight
	got := updated.(dashboardModel)
	updated, _ = got.Update(runeKey('u')) // user opens the page modal
	got = updated.(dashboardModel)
	if got.mode != modeUpdatePage {
		t.Fatalf("mode = %v, want the page modal open", got.mode)
	}

	updated, _ = got.Update(opDoneMsg{op: opProgress, info: "Progress +10 → page 40", markDirty: true})
	got = updated.(dashboardModel)

	if got.mode != modeUpdatePage {
		t.Fatal("a progress result started before the modal opened must not close it")
	}
	if got.pageSubmitting {
		t.Fatal("the modal has not submitted anything yet")
	}
}

func TestPageModalEnterIsGuardedWhileInFlight(t *testing.T) {
	m := newTestDashboard()
	m.loaded = true
	m.section = sectionReading
	m.readingBooks = []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}}
	m.refreshListItems()

	updated, _ := m.Update(runeKey('+'))
	got := updated.(dashboardModel)
	updated, _ = got.Update(runeKey('u'))
	got = updated.(dashboardModel)
	got.pageInput.SetValue("120")

	updated, cmd := got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(dashboardModel)

	if cmd != nil {
		t.Fatal("enter must not submit a page update while one is in flight")
	}
	if got.pageSubmitting {
		t.Fatal("the refused submission must not mark the modal as submitting")
	}
	if got.mode != modeUpdatePage {
		t.Fatal("the modal should stay open after a refused submission")
	}
	if got.infoMsg == "" {
		t.Fatal("the refused submission should say why")
	}
}

func TestPageModalClosesOnItsOwnResult(t *testing.T) {
	m := newTestDashboard()
	m.loaded = true
	m.mode = modeUpdatePage
	m.pendingBookID = 1
	m.pageInput.SetValue("120")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(dashboardModel)
	if cmd == nil || !got.pageSubmitting {
		t.Fatal("enter should submit the page update")
	}

	updated, _ = got.Update(opDoneMsg{op: opProgress, info: "Progress updated to page 120", markDirty: true})
	got = updated.(dashboardModel)

	if got.mode != modeLibrary || got.pageSubmitting {
		t.Fatal("the modal's own result should close it")
	}
}

func TestSearchInsertModeTypesQuestionMark(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionSearch
	m.searchSub = searchSubInput
	m.enterSearchInsertMode()

	updated, _ := m.Update(runeKey('?'))
	got := updated.(dashboardModel)

	if got.showHelp {
		t.Fatal("? should be typed in insert mode, not open help")
	}
	if got.searchInput.Value() != "?" {
		t.Fatalf("search input = %q, want ?", got.searchInput.Value())
	}
}

func TestSectionFocusResizesLists(t *testing.T) {
	m := newTestDashboard()
	m.width, m.height = 100, 44
	m.setSection(sectionReading)

	readingFocused := m.readingList.Height()

	m.nextSection() // Oku takes the focus, and with it the extra rows.
	if m.section != sectionOku {
		t.Fatalf("section = %v, want %v", m.section, sectionOku)
	}
	if m.readingList.Height() >= readingFocused {
		t.Fatalf("reading list height = %d, want less than the focused %d", m.readingList.Height(), readingFocused)
	}

	heights := m.leftSectionHeights(m.rightPanelContentHeight())
	if want := max(1, heights[sectionReading]-3); m.readingList.Height() != want {
		t.Fatalf("reading list height = %d, want %d", m.readingList.Height(), want)
	}
	if want := max(1, heights[sectionOku]-3); m.okuList.Height() != want {
		t.Fatalf("oku list height = %d, want %d", m.okuList.Height(), want)
	}
}

func TestSlashFocusResizesLists(t *testing.T) {
	m := newTestDashboard()
	m.width, m.height = 100, 44
	m.setSection(sectionReading)

	updated, _ := m.Update(runeKey('/'))
	got := updated.(dashboardModel)

	heights := got.leftSectionHeights(got.rightPanelContentHeight())
	if want := max(1, heights[sectionReading]-3); got.readingList.Height() != want {
		t.Fatalf("reading list height after / = %d, want %d", got.readingList.Height(), want)
	}
}

func TestReviewModalIsReadOnlyWhileSaving(t *testing.T) {
	m := newTestDashboard()
	m.loaded = true
	m.openReviewRatingModal(model.UserBook{Book: model.Book{ID: 42, Title: "Dune"}})
	m.reviewRatingInput.SetValue("3")
	m.reviewTextInput.SetValue("Strong first half.")

	updated, _ := m.updateReviewRatingMode(tea.KeyMsg{Type: tea.KeyCtrlS})
	saving := updated.(dashboardModel)
	pendingSeq := saving.reviewSeq

	updated, _ = saving.Update(runeKey('x'))
	typed := updated.(dashboardModel)
	if typed.reviewTextInput.Value() != "Strong first half." {
		t.Fatalf("review text = %q, want it read-only while saving", typed.reviewTextInput.Value())
	}

	// Cancelling drops the pending result instead of reopening the modal.
	updated, _ = typed.Update(tea.KeyMsg{Type: tea.KeyEsc})
	cancelled := updated.(dashboardModel)
	if cancelled.mode != modeLibrary || cancelled.reviewBook != nil {
		t.Fatal("esc should close the modal even while saving")
	}

	updated, _ = cancelled.Update(opDoneMsg{op: opReview, seq: pendingSeq, err: errors.New("save failed")})
	late := updated.(dashboardModel)
	if late.mode != modeLibrary {
		t.Fatal("a result for a cancelled modal must not reopen it")
	}
	if late.reviewErr != "" {
		t.Fatalf("reviewErr = %q, want the cancelled save reported through the status bar", late.reviewErr)
	}
	if late.errMsg != "save failed" {
		t.Fatalf("errMsg = %q, want the failure reported in the status bar", late.errMsg)
	}
}

func TestSearchErrorClearsLoadingTitle(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionSearch
	m.searchInput.SetValue("dune")
	if cmd := m.submitSearch(); cmd == nil {
		t.Fatal("submitSearch() returned nil cmd")
	}

	updated, _ := m.Update(searchLoadedMsg{
		query: "dune",
		mode:  model.SearchModeBook,
		seq:   m.searchSeq,
		err:   errors.New("network down"),
	})
	got := updated.(dashboardModel)

	if strings.Contains(got.searchList.Title, "loading") {
		t.Fatalf("searchList title = %q, want the loading state cleared", got.searchList.Title)
	}
	if got.searchLoading {
		t.Fatal("a failed search should clear searchLoading")
	}
}

// renderedDashboard builds a loaded dashboard of the given size with a couple
// of books on each shelf, ready to render.
func renderedDashboard(w, h int) dashboardModel {
	m := newTestDashboard()
	m.loaded = true
	m.readingBooks = []model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune", Pages: 412}, CurrentPage: 120},
		{Book: model.Book{ID: 2, Title: "Foundation", Pages: 255}},
	}
	m.okuBooks = []model.UserBook{
		{Book: model.Book{ID: 3, Title: "Meditations"}},
	}
	m.refreshListItems()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(dashboardModel)
}

func TestViewFillsTerminalExactly(t *testing.T) {
	sections := []focusSection{
		sectionIntro, sectionReading, sectionOku,
		sectionSearch, sectionStats, sectionTimer,
	}
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		w, h := size[0], size[1]
		for _, section := range sections {
			m := renderedDashboard(w, h)
			m.setSection(section)

			// The layout must fill the screen on its own, not be padded
			// into it by View's final clamp.
			if got := len(strings.Split(m.frame(), "\n")); got != h {
				t.Fatalf("%dx%d section %v: frame has %d lines, want %d", w, h, section, got, h)
			}

			lines := strings.Split(m.View(), "\n")
			if len(lines) != h {
				t.Fatalf("%dx%d section %v: view has %d lines, want %d", w, h, section, len(lines), h)
			}
			for i, line := range lines {
				if got := lipgloss.Width(line); got > w {
					t.Fatalf("%dx%d section %v: line %d is %d wide, want <= %d", w, h, section, i, got, w)
				}
			}
		}
	}
}
