package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
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
	if !strings.Contains(strings.ToLower(m.toast.text), "searching for") {
		t.Fatalf("toast %q does not include searching feedback", m.toast.text)
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
	m.inflight = 0
	m.searchLoading = false

	m.submitSearch()
	if m.isLoading() || m.searchLoading {
		t.Fatal("submitSearch() should not search for an empty query")
	}
	if m.toast.level != toastError || m.toast.text == "" {
		t.Fatalf("toast = %+v, want a validation error for the empty query", m.toast)
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
	if !strings.Contains(got.toast.text, "loaded 1 results") {
		t.Fatalf("toast = %q, expected loaded-count feedback", got.toast.text)
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

	updated, _ := m.updateLibraryMode(runeKey('t'))
	got := updated.(dashboardModel)

	if got.isLoading() {
		t.Fatal("timer start should open selection first, got an immediate operation")
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
	if got.toast.text != "Saving review..." {
		t.Fatalf("toast = %q, want Saving review...", got.toast.text)
	}
}

func TestTimerSelectEnterClampsStaleIndex(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionTimer
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
	if got.toast.text != "Timer started — Dune" {
		t.Fatalf("toast = %q, want the timer info", got.toast.text)
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

	updated, _ = got.Update(runeKey('+'))
	got = updated.(dashboardModel)
	if got.inflight != 1 {
		t.Fatalf("inflight = %d: a second + while an update is in flight must not fire, it would lose one", got.inflight)
	}
	if got.toast.text == "" || got.toast.level != toastWarn {
		t.Fatalf("toast = %+v, want the refused update to say why", got.toast)
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
	if got.toast.text != "Progress +10 → page 40" {
		t.Fatalf("toast = %q, want the unrelated result reported", got.toast.text)
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

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got = updated.(dashboardModel)

	if got.inflight != 1 {
		t.Fatalf("inflight = %d: enter must not submit a page update while one is in flight", got.inflight)
	}
	if got.pageSubmitting {
		t.Fatal("the refused submission must not mark the modal as submitting")
	}
	if got.mode != modeUpdatePage {
		t.Fatal("the modal should stay open after a refused submission")
	}
	if got.toast.text == "" {
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
	if late.toast.text != "save failed" || late.toast.level != toastError {
		t.Fatalf("toast = %+v, want the failure reported in the status bar", late.toast)
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

func TestHelpBarTruncatesToWidth(t *testing.T) {
	sections := []focusSection{
		sectionIntro, sectionReading, sectionOku,
		sectionSearch, sectionStats, sectionTimer,
	}
	for _, w := range []int{60, 79, 80, 100, 120, 200} {
		for _, section := range sections {
			m := renderedDashboard(w, 40)
			m.setSection(section)

			bar := m.contextHelpBar()
			if strings.Contains(bar, "\n") {
				t.Fatalf("width %d section %v: help bar wrapped: %q", w, section, bar)
			}
			if got := lipgloss.Width(bar); got > w {
				t.Fatalf("width %d section %v: help bar is %d wide", w, section, got)
			}

			// A truncated bar must end in the ellipsis, never in a dangling
			// separator or half a word.
			plain := strings.TrimRight(stripANSI(bar), " ")
			if strings.Contains(plain, "\u2026") && !strings.HasSuffix(plain, "\u2026") {
				t.Fatalf("width %d section %v: %q does not end at the ellipsis", w, section, plain)
			}
			if !strings.Contains(plain, "\u2026") {
				// Nothing was dropped, so every hint has to be there.
				for _, b := range m.helpBindings() {
					if !strings.Contains(plain, b.Help().Key) {
						t.Fatalf("width %d section %v: %q is missing %q", w, section, plain, b.Help().Key)
					}
				}
			}
		}
	}
}

func TestHelpHintIsNeverTheHintThatGetsDropped(t *testing.T) {
	for _, w := range []int{60, 79, 80, 100, 120} {
		m := renderedDashboard(w, 40)
		m.setSection(sectionReading)
		if got := stripANSI(m.contextHelpBar()); !strings.Contains(got, "? help") {
			t.Fatalf("width %d: %q should always keep the help hint", w, got)
		}
	}
}

func TestSearchInsertModeDoesNotAdvertiseHelp(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setSection(sectionSearch)
	m.enterSearchInsertMode()

	if got := stripANSI(m.contextHelpBar()); strings.Contains(got, "? help") {
		t.Fatalf("help bar = %q, but ? types a literal in insert mode", got)
	}
}

func TestRecentSearchesRoundTrip(t *testing.T) {
	queries := []string{"dune", "Dune", "  ", "east of eden", "clean code"}

	raw, err := encodeRecentSearches(queries)
	if err != nil {
		t.Fatalf("encodeRecentSearches() error = %v", err)
	}

	got := decodeRecentSearches(raw)
	want := []string{"dune", "east of eden", "clean code"}
	if len(got) != len(want) {
		t.Fatalf("decodeRecentSearches() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("decodeRecentSearches()[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if decodeRecentSearches("") != nil {
		t.Fatal("an empty state value should decode to no history")
	}
	if decodeRecentSearches("{not json") != nil {
		t.Fatal("a corrupt state value should decode to no history")
	}

	long := make([]string, 0, maxRecentSearches+5)
	for i := 0; i < maxRecentSearches+5; i++ {
		long = append(long, string(rune('a'+i)))
	}
	raw, err = encodeRecentSearches(long)
	if err != nil {
		t.Fatalf("encodeRecentSearches() error = %v", err)
	}
	if got := decodeRecentSearches(raw); len(got) != maxRecentSearches {
		t.Fatalf("history kept %d queries, want the %d most recent", len(got), maxRecentSearches)
	}
}

func TestSearchSuggestionsComeFromHistoryOnly(t *testing.T) {
	m := newTestDashboard()
	if got := m.searchInput.AvailableSuggestions(); len(got) != 0 {
		t.Fatalf("a fresh dashboard suggests %#v, want nothing until the user searches", got)
	}

	// A nil app must not panic on the way to the (skipped) save.
	if cmd := m.addRecentSearchQuery("east of eden"); cmd != nil {
		t.Fatal("addRecentSearchQuery() should not try to save without a store")
	}
	m.updateSearchSuggestions()

	got := m.searchInput.AvailableSuggestions()
	if len(got) != 1 || got[0] != "east of eden" {
		t.Fatalf("suggestions = %#v, want the query just searched for", got)
	}
}

func TestLocalDataMergesStoredRecentSearches(t *testing.T) {
	m := newTestDashboard()
	m.inflight = 1
	m.addRecentSearchQuery("dune")

	updated, _ := m.Update(localDataLoadedMsg{recentSearches: []string{"dune", "clean code"}})
	got := updated.(dashboardModel)

	want := []string{"dune", "clean code"}
	if len(got.recentSearches) != len(want) {
		t.Fatalf("recentSearches = %#v, want %#v", got.recentSearches, want)
	}
	for i := range want {
		if got.recentSearches[i] != want[i] {
			t.Fatalf("recentSearches[%d] = %q, want %q", i, got.recentSearches[i], want[i])
		}
	}
}

func TestTimerStartsAndStopsFromReadingList(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setSection(sectionReading)

	updated, cmd := m.updateLibraryMode(runeKey('t'))
	started := updated.(dashboardModel)
	if cmd == nil {
		t.Fatal("t in the Reading list should start a timer for the selection")
	}
	if !started.isLoading() {
		t.Fatal("starting a timer should be counted as in flight")
	}
	if started.timerSelecting {
		t.Fatal("the Reading list already has a selection: no picker")
	}

	// With a timer running, t stops it.
	running := renderedDashboard(100, 40)
	running.setSection(sectionReading)
	running.timerState = &model.TimerState{BookID: 1, StartedAt: time.Now()}

	updated, cmd = running.updateLibraryMode(runeKey('t'))
	if cmd == nil {
		t.Fatal("t should stop the running timer")
	}
	if updated.(dashboardModel).timerSelecting {
		t.Fatal("stopping a timer must not open the picker")
	}
}

func TestTimerKeyOutsideReadingExplainsItself(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setSection(sectionOku)

	updated, _ := m.updateLibraryMode(runeKey('t'))
	got := updated.(dashboardModel)
	if got.isLoading() {
		t.Fatal("t in the Oku list has no book to track, so it should start nothing")
	}
	if !strings.Contains(got.toast.text, "Reading list") {
		t.Fatalf("toast = %q, want it to point at the Reading list", got.toast.text)
	}
}

func TestEnterDoesNotChangeStatus(t *testing.T) {
	for _, section := range []focusSection{sectionReading, sectionOku} {
		m := renderedDashboard(100, 40)
		m.setSection(section)

		updated, _ := m.updateLibraryMode(tea.KeyMsg{Type: tea.KeyEnter})
		got := updated.(dashboardModel)
		if got.isLoading() {
			t.Fatalf("section %v: Enter started an operation", section)
		}
		if got.toast.level != toastInfo {
			t.Fatalf("section %v: toast = %+v, want no error", section, got.toast)
		}
		if got.toast.text == "" {
			t.Fatalf("section %v: Enter should name the book it brought into the detail pane", section)
		}
	}
}

func TestPageModalShowsTitleAndKeepsFormatHint(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setSection(sectionReading)

	updated, _ := m.updateLibraryMode(runeKey('u'))
	got := updated.(dashboardModel)

	if got.mode != modeUpdatePage {
		t.Fatalf("mode after u = %v, want %v", got.mode, modeUpdatePage)
	}
	if got.pageInput.Value() != "" {
		t.Fatalf("page input pre-filled with %q, want it empty", got.pageInput.Value())
	}
	if got.pageInput.Placeholder != "370 or +10 or -5" {
		t.Fatalf("placeholder = %q, want the format hint", got.pageInput.Placeholder)
	}

	prompt := got.pagePrompt()
	if !strings.Contains(prompt, "Dune") {
		t.Fatalf("prompt %q does not name the book", prompt)
	}
	if !strings.Contains(prompt, "current: 120/412") {
		t.Fatalf("prompt %q does not show where the book stands", prompt)
	}

	// The prompt is two rows taller than the help bar it replaces, and the
	// layout has to give those rows back.
	if lines := strings.Split(got.frame(), "\n"); len(lines) != 40 {
		t.Fatalf("page prompt frame has %d lines, want 40", len(lines))
	}
}

func TestHelpModalFitsTheTerminalAndScrolls(t *testing.T) {
	for _, h := range []int{24, 40} {
		m := renderedDashboard(80, h)

		updated, _ := m.Update(runeKey('?'))
		opened := updated.(dashboardModel)
		if !opened.showHelp {
			t.Fatalf("height %d: ? should open the help modal", h)
		}
		if lines := strings.Split(opened.frame(), "\n"); len(lines) != h {
			t.Fatalf("height %d: help frame has %d lines, want %d", h, len(lines), h)
		}
		if opened.helpViewport.TotalLineCount() <= opened.helpViewport.Height {
			t.Fatalf("height %d: the help body should be taller than the window", h)
		}
		if !strings.Contains(opened.renderHelpModal(), "j/k scroll") {
			t.Fatalf("height %d: an overflowing help modal should say it scrolls", h)
		}

		updated, _ = opened.Update(runeKey('j'))
		scrolled := updated.(dashboardModel)
		if scrolled.helpViewport.YOffset != 1 {
			t.Fatalf("height %d: j should scroll the body, YOffset = %d", h, scrolled.helpViewport.YOffset)
		}

		updated, _ = scrolled.Update(runeKey('k'))
		if got := updated.(dashboardModel).helpViewport.YOffset; got != 0 {
			t.Fatalf("height %d: k should scroll back, YOffset = %d", h, got)
		}

		updated, _ = scrolled.Update(runeKey('?'))
		if updated.(dashboardModel).showHelp {
			t.Fatalf("height %d: ? should close the help modal", h)
		}
	}
}

func TestIgnoreAsksBeforeItChangesTheStatus(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setSection(sectionReading)

	updated, cmd := m.Update(runeKey('x'))
	asked := updated.(dashboardModel)
	if cmd != nil {
		t.Fatal("x should ask before it changes the status")
	}
	if !asked.confirm.Active {
		t.Fatal("x should open the confirmation")
	}
	if !strings.Contains(asked.confirm.Message, "Dune") {
		t.Fatalf("confirm message = %q, want it to name the book", asked.confirm.Message)
	}
	if !strings.Contains(asked.confirm.Message, model.StatusIgnored.Label()) {
		t.Fatalf("confirm message = %q, want it to name the new status", asked.confirm.Message)
	}

	updated, _ = asked.Update(runeKey('n'))
	cancelled := updated.(dashboardModel)
	if cancelled.confirm.Active || cancelled.isLoading() {
		t.Fatal("n should drop the change without running anything")
	}

	updated, cmd = asked.Update(runeKey('y'))
	confirmed := updated.(dashboardModel)
	if cmd == nil {
		t.Fatal("y should run the status change")
	}
	if confirmed.confirm.Active {
		t.Fatal("the confirmation should close once it is answered")
	}
	if !confirmed.isLoading() {
		t.Fatal("the confirmed change should be counted as in flight")
	}

	// Esc is the same answer as n.
	updated, _ = asked.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := updated.(dashboardModel); got.confirm.Active || got.isLoading() {
		t.Fatal("esc should drop the change")
	}
}

func TestDidNotFinishAsksToo(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setSection(sectionReading)

	updated, cmd := m.Update(runeKey('d'))
	got := updated.(dashboardModel)
	if cmd != nil || !got.confirm.Active {
		t.Fatal("d should ask before closing the read")
	}
	if !strings.Contains(got.confirm.Message, model.StatusDidNotFinish.Label()) {
		t.Fatalf("confirm message = %q, want it to name the new status", got.confirm.Message)
	}
}

func TestConfirmationDoesNotSwallowAsyncResults(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setSection(sectionReading)

	updated, _ := m.Update(runeKey('x'))
	asked := updated.(dashboardModel)
	asked.inflight = 1

	updated, _ = asked.Update(libraryLoadedMsg{
		reading: []model.UserBook{{Book: model.Book{ID: 9, Title: "Ubik"}}},
	})
	got := updated.(dashboardModel)

	if !got.confirm.Active {
		t.Fatal("an async result must not close the confirmation")
	}
	if len(got.readingBooks) != 1 || got.readingBooks[0].Book.Title != "Ubik" {
		t.Fatalf("readingBooks = %#v, want the load applied behind the confirmation", got.readingBooks)
	}
	if got.isLoading() {
		t.Fatal("the finished load should have released its slot")
	}
}

// stripANSI removes the escape sequences so a test can look at the glyphs.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func TestLongOrMultilineMessagesKeepTheFrameIntact(t *testing.T) {
	messages := []string{
		strings.Repeat("network unreachable ", 10),
		"Post \"https://api.hardcover.app/v1/graphql\":\ndial tcp: lookup failed\nand a third line",
	}
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		w, h := size[0], size[1]
		for i, msg := range messages {
			m := renderedDashboard(w, h)
			m.setSection(sectionReading)
			m.showToast(toastError, msg)

			frame := m.frame()
			lines := strings.Split(frame, "\n")
			if len(lines) != h {
				t.Fatalf("%dx%d message %d: frame has %d lines, want %d", w, h, i, len(lines), h)
			}
			for n, line := range lines {
				if got := lipgloss.Width(line); got > w {
					t.Fatalf("%dx%d message %d: line %d is %d wide", w, h, i, n, got)
				}
			}
			if last := stripANSI(lines[h-1]); !strings.Contains(last, "? help") {
				t.Fatalf("%dx%d message %d: last line is %q, want the help bar", w, h, i, last)
			}
			if strings.Contains(stripANSI(lines[0]), "dial tcp") &&
				strings.Contains(stripANSI(lines[1]), "lookup failed") {
				t.Fatalf("%dx%d message %d: the message wrapped the status bar", w, h, i)
			}
		}
	}
}

func TestListCardShowsHowFarDownTheListIs(t *testing.T) {
	m := renderedDashboard(120, 40)
	books := make([]model.UserBook, 0, 30)
	for i := 0; i < 30; i++ {
		books = append(books, model.UserBook{Book: model.Book{ID: 100 + i, Title: fmt.Sprintf("Book %d", i)}})
	}
	m.okuBooks = books
	m.refreshListItems()
	m.setSection(sectionOku)

	badge := m.listOverflowBadge(sectionOku)
	if badge != "1/30" {
		t.Fatalf("overflow badge = %q, want 1/30", badge)
	}
	if !strings.Contains(stripANSI(m.frame()), badge) {
		t.Fatal("the Oku card should show the overflow badge")
	}

	// A list that fits says nothing.
	m.okuBooks = books[:1]
	m.refreshListItems()
	if got := m.listOverflowBadge(sectionOku); got != "" {
		t.Fatalf("overflow badge = %q, want none when the list fits", got)
	}

	// The focused card's rows are padded out to its full width, so the badge
	// has to overwrite the tail rather than be appended to it.
	small := renderedDashboard(80, 24)
	small.readingBooks = books[:5]
	small.refreshListItems()
	small.setSection(sectionReading)
	if got := small.listOverflowBadge(sectionReading); got != "1/5" {
		t.Fatalf("focused overflow badge = %q, want 1/5", got)
	}
	if !strings.Contains(stripANSI(small.frame()), "1/5") {
		t.Fatal("the focused Reading card should show the overflow badge")
	}
}

func TestProgressRowFitsTheDetailPane(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		m := renderedDashboard(size[0], size[1])
		m.setSection(sectionReading)

		w := m.rightPanelContentWidth()
		for _, line := range strings.Split(stripANSI(m.detailsView(w)), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Fatalf("%dx%d: detail line %q is %d wide, want <= %d", size[0], size[1], line, got, w)
			}
		}
		if !strings.Contains(stripANSI(m.frame()), "29%") {
			t.Fatalf("%dx%d: the progress percentage should not be clipped", size[0], size[1])
		}
	}
}

func TestTimerSectionStopsWithT(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setSection(sectionTimer)
	m.timerState = &model.TimerState{BookID: 1, StartedAt: time.Now()}

	updated, cmd := m.updateLibraryMode(runeKey('t'))
	if cmd == nil {
		t.Fatal("t should stop the running timer in the Timer section too")
	}
	if updated.(dashboardModel).timerSelecting {
		t.Fatal("stopping must not open the book picker")
	}
}

func TestSecondTimerPressIsGuardedWhileInFlight(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setSection(sectionReading)

	updated, cmd := m.updateLibraryMode(runeKey('t'))
	started := updated.(dashboardModel)
	if cmd == nil {
		t.Fatal("t should start a timer")
	}

	updated, _ = started.updateLibraryMode(runeKey('t'))
	if updated.(dashboardModel).inflight != started.inflight {
		t.Fatal("a second t before the first result returns must not start another session")
	}
	if !strings.Contains(updated.(dashboardModel).toast.text, "in flight") {
		t.Fatalf("toast = %q, want the in-flight notice", updated.(dashboardModel).toast.text)
	}
}

func TestSearchHeaderNamesTheResultsOnScreen(t *testing.T) {
	m := newTestDashboard()
	m.section = sectionSearch
	m.searchInput.SetValue("dune")
	m.inflight = 1

	updated, _ := m.Update(searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}, {ID: 2, Title: "Dune Messiah"}},
		query:   "dune",
		mode:    model.SearchModeBook,
		seq:     m.searchSeq,
	})
	got := updated.(dashboardModel)
	if got.searchList.Title != "BOOK Results (2)" {
		t.Fatalf("title = %q, want BOOK Results (2)", got.searchList.Title)
	}

	// Switching the mode does not rename results fetched in the old one.
	got.setSearchQueryMode(model.SearchModeAuthor)
	if got.searchList.Title != "BOOK Results (2)" {
		t.Fatalf("title after switching mode = %q, want the results' own mode", got.searchList.Title)
	}

	// A failed search leaves the previous results, and their count, named.
	got.inflight = 1
	got.searchLoading = true
	updated, _ = got.Update(searchLoadedMsg{
		query: "dune",
		mode:  model.SearchModeAuthor,
		seq:   got.searchSeq,
		err:   errors.New("network down"),
	})
	failed := updated.(dashboardModel)
	if failed.searchList.Title != "BOOK Results (2)" {
		t.Fatalf("title after a failed search = %q, want the results still on screen", failed.searchList.Title)
	}
}

// withColorProfile renders in colour for the duration of a test. Tests have
// no terminal, so lipgloss would otherwise strip every colour and a light and
// a dark render would be the same bytes.
func withColorProfile(t *testing.T) {
	t.Helper()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(termenv.Ascii)
		lipgloss.SetHasDarkBackground(true)
	})
}

func TestThemeResolvesDistinctColoursForLightAndDark(t *testing.T) {
	withColorProfile(t)

	th := defaultTheme()
	colours := map[string]lipgloss.AdaptiveColor{
		"accent": th.accent, "heading": th.heading, "text": th.text,
		"textMuted": th.textMuted, "textDim": th.textDim, "border": th.border,
		"borderFocused": th.borderFocused, "surface": th.surface,
		"success": th.success, "warning": th.warning, "error": th.error,
		"heat1": th.heat1, "heat2": th.heat2, "heat3": th.heat3, "heat4": th.heat4,
	}
	for name, c := range colours {
		if c.Light == "" || c.Dark == "" {
			t.Fatalf("%s: both sides of the theme must be set, got %+v", name, c)
		}
		if c.Light == c.Dark {
			t.Fatalf("%s: light and dark are the same value %q", name, c.Light)
		}

		style := lipgloss.NewStyle().Foreground(c)
		lipgloss.SetHasDarkBackground(true)
		dark := style.Render("x")
		lipgloss.SetHasDarkBackground(false)
		light := style.Render("x")
		if dark == light {
			t.Fatalf("%s: renders the same on a light and a dark terminal: %q", name, dark)
		}
	}

	// The heat ramp has four distinct steps on each side.
	for _, side := range []struct {
		name string
		pick func(lipgloss.AdaptiveColor) string
	}{
		{"dark", func(c lipgloss.AdaptiveColor) string { return c.Dark }},
		{"light", func(c lipgloss.AdaptiveColor) string { return c.Light }},
	} {
		seen := map[string]bool{}
		for _, c := range []lipgloss.AdaptiveColor{th.heat1, th.heat2, th.heat3, th.heat4} {
			if seen[side.pick(c)] {
				t.Fatalf("%s heat ramp repeats %q", side.name, side.pick(c))
			}
			seen[side.pick(c)] = true
		}
	}
}

func TestApplyThemeSetting(t *testing.T) {
	withColorProfile(t)

	if err := applyThemeSetting("light"); err != nil {
		t.Fatalf("applyThemeSetting(light) error = %v", err)
	}
	if lipgloss.HasDarkBackground() {
		t.Fatal("theme = light should pin a light background")
	}
	if err := applyThemeSetting("Dark"); err != nil {
		t.Fatalf("applyThemeSetting(Dark) error = %v", err)
	}
	if !lipgloss.HasDarkBackground() {
		t.Fatal("theme = dark should pin a dark background")
	}
	for _, ok := range []string{"", "auto", " AUTO "} {
		if err := applyThemeSetting(ok); err != nil {
			t.Fatalf("applyThemeSetting(%q) error = %v, want none", ok, err)
		}
	}
	if err := applyThemeSetting("solarized"); err == nil {
		t.Fatal("an unknown theme should be reported, not ignored")
	}
}

// keyMsgFor turns a binding's key name into the KeyMsg Bubble Tea would send
// for it.
func keyMsgFor(t *testing.T, name string) tea.KeyMsg {
	t.Helper()
	for _, kt := range []tea.KeyType{
		tea.KeyEnter, tea.KeyEsc, tea.KeyTab, tea.KeyShiftTab,
		tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight,
		tea.KeyHome, tea.KeyEnd, tea.KeyPgUp, tea.KeyPgDown,
		tea.KeyCtrlC, tea.KeyCtrlD, tea.KeyCtrlU, tea.KeyCtrlS,
	} {
		if (tea.KeyMsg{Type: kt}).String() == name {
			return tea.KeyMsg{Type: kt}
		}
	}
	runes := []rune(name)
	if len(runes) != 1 {
		t.Fatalf("no KeyMsg for key %q", name)
	}
	return runeKey(runes[0])
}

// TestEveryAdvertisedBindingIsHandled walks every focus the dashboard can
// have, takes the bindings its help advertises, and presses each of their
// keys: every one has to change the screen or return a command. A binding
// that is listed but not dispatched fails here.
func TestEveryAdvertisedBindingIsHandled(t *testing.T) {
	books := func(ids ...int) []model.UserBook {
		out := make([]model.UserBook, 0, len(ids))
		for _, id := range ids {
			out = append(out, model.UserBook{Book: model.Book{ID: id, Title: fmt.Sprintf("Book %d", id), Pages: 300}, CurrentPage: 100})
		}
		return out
	}
	// Three books on each shelf and a cursor in the middle, so both up and
	// down move; the demo stats so the stats page is tall enough to scroll.
	base := func(w, h int) dashboardModel {
		m := renderedDashboard(w, h)
		m.readingBooks = books(1, 2, 3)
		m.okuBooks = books(4, 5, 6)
		m.refreshListItems()
		m.readingList.Select(1)
		m.okuList.Select(1)
		m.readingStats, _ = demoLocalData()
		return m
	}
	withResults := func(m dashboardModel) dashboardModel {
		m.searchBooks = []model.SearchResult{{ID: 7, Title: "Dune"}, {ID: 8, Title: "Dune Messiah"}, {ID: 9, Title: "Children of Dune"}}
		m.refreshSearchResultItems()
		m.searchList.Select(1)
		m.searchInput.SetValue("dune")
		return m
	}
	inSection := func(s focusSection) func() dashboardModel {
		return func() dashboardModel {
			m := base(120, 40)
			m.setSection(s)
			return m
		}
	}
	confirmWithCursor := func(cursor int) func() dashboardModel {
		return func() dashboardModel {
			m := inSection(sectionReading)()
			updated, _ := m.Update(runeKey('x'))
			m = updated.(dashboardModel)
			m.confirm.Cursor = cursor
			return m
		}
	}

	states := []struct {
		name string
		// variants are arrangements of the same focus; a key passes when it
		// does something in any of them (a two-button dialog cannot move
		// left and right from one cursor).
		variants []func() dashboardModel
	}{
		{"intro", []func() dashboardModel{inSection(sectionIntro)}},
		{"reading", []func() dashboardModel{inSection(sectionReading)}},
		{"oku", []func() dashboardModel{inSection(sectionOku)}},
		{"search input, normal mode", []func() dashboardModel{func() dashboardModel {
			m := withResults(inSection(sectionSearch)())
			m.searchSub = searchSubInput
			m.enterSearchNormalMode()
			return m
		}}},
		{"search input, insert mode", []func() dashboardModel{func() dashboardModel {
			m := withResults(inSection(sectionSearch)())
			m.searchSub = searchSubInput
			m.enterSearchInsertMode()
			return m
		}}},
		{"search results", []func() dashboardModel{func() dashboardModel {
			m := withResults(inSection(sectionSearch)())
			m.searchSub = searchSubResults
			m.enterSearchNormalMode()
			return m
		}}},
		{"stats", []func() dashboardModel{func() dashboardModel {
			// One row down: the page can still scroll either way.
			m := inSection(sectionStats)()
			m.statsScroll = 1
			return m
		}}},
		{"timer, idle", []func() dashboardModel{inSection(sectionTimer)}},
		{"timer, picking a book", []func() dashboardModel{func() dashboardModel {
			m := inSection(sectionTimer)()
			m.timerSelecting = true
			m.timerSelectIdx = 1
			return m
		}}},
		{"timer, running", []func() dashboardModel{func() dashboardModel {
			m := inSection(sectionTimer)()
			m.timerState = &model.TimerState{BookID: 1, StartedAt: time.Now()}
			return m
		}}},
		{"help modal", []func() dashboardModel{func() dashboardModel {
			// Short enough that the body scrolls, scrolled one row so every
			// direction has somewhere to go.
			m := base(80, 24)
			m.setSection(sectionReading)
			m.openHelp()
			m.helpViewport.SetYOffset(1)
			return m
		}}},
		{"confirm", []func() dashboardModel{confirmWithCursor(1), confirmWithCursor(0)}},
		{"page prompt", []func() dashboardModel{func() dashboardModel {
			m := inSection(sectionReading)()
			m.openPageModal(m.readingBooks[1])
			return m
		}}},
		{"review modal", []func() dashboardModel{func() dashboardModel {
			m := inSection(sectionReading)()
			m.openReviewRatingModal(m.readingBooks[1])
			m.reviewRatingInput.SetValue("3")
			return m
		}}},
		{"undo on offer", []func() dashboardModel{func() dashboardModel {
			m := inSection(sectionOku)()
			m.showUndoToast("Moved 'Book 5' to Read", undoAction{op: opStatus, bookID: 5,
				fromStatus: model.StatusRead, toStatus: model.StatusWantToRead})
			return m
		}}},
	}

	for _, state := range states {
		groups := state.variants[0]().activeKeys().FullHelp()
		if len(groups) == 0 {
			t.Fatalf("%s: no bindings are advertised", state.name)
		}
		for _, group := range groups {
			for _, b := range group {
				for _, name := range b.Keys() {
					msg := keyMsgFor(t, name)
					handled := false
					for _, arrange := range state.variants {
						m := arrange()
						before := stripANSI(m.frame())
						updated, cmd := m.Update(msg)
						if cmd != nil || stripANSI(updated.(dashboardModel).frame()) != before {
							handled = true
							break
						}
					}
					if !handled {
						t.Errorf("%s: %q (%s: %s) is advertised but does nothing", state.name, name, b.Help().Key, b.Help().Desc)
					}
				}
			}
		}
	}
}

// TestHelpModalListsEveryGroupWithTheActiveOnesFirst checks the modal still
// teaches the other sections' keys: from Reading it names the search keys,
// dimmed, after the groups that apply.
func TestHelpModalListsEveryGroupWithTheActiveOnesFirst(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setSection(sectionReading)
	body := stripANSI(m.helpModalBody())

	for _, want := range []string{"Actions", "Navigation", "General", "Confirm", "Review", "Timer", "Data",
		"insert", "cycle mode", "book / author / genre", "undo the last change", "stop timer"} {
		if !strings.Contains(body, want) {
			t.Fatalf("help body is missing %q:\n%s", want, body)
		}
	}
	// Confirm and Review have no live key from Reading, so they come after
	// every group that does.
	if strings.Index(body, "Confirm") < strings.Index(body, "General") {
		t.Fatalf("inactive groups should follow the active ones:\n%s", body)
	}
	// Dimming is the only difference, so the raw render of a live key and a
	// dead one must differ.
	raw := m.helpModalBody()
	live := modalKeyStyle.Width(12).Render("u")
	if !strings.Contains(raw, live) {
		t.Fatal("a live key should be drawn in the key style")
	}
	if strings.Contains(raw, modalKeyStyle.Width(12).Render("i")) {
		t.Fatal("a key the focus does not understand should not be drawn as live")
	}
}

// TestOverloadedKeysMatchTheirHelp pins the help-modal labels of keys that
// mean different things in different places to what the handler does there.
func TestOverloadedKeysMatchTheirHelp(t *testing.T) {
	modalRows := func(m dashboardModel) string { return stripANSI(m.helpModalBody()) }

	// Over the search results h goes back to the input, so the section hint
	// must not claim it.
	m := renderedDashboard(120, 40)
	m.setSection(sectionSearch)
	m.searchBooks = []model.SearchResult{{ID: 1, Title: "Dune"}}
	m.refreshSearchResultItems()
	m.searchSub = searchSubResults
	m.enterSearchNormalMode()

	updated, _ := m.Update(runeKey('h'))
	if got := updated.(dashboardModel); got.searchSub != searchSubInput || got.section != sectionSearch {
		t.Fatalf("h over the results: searchSub=%v section=%v, want back to the input", got.searchSub, got.section)
	}
	k := m.activeKeys()
	for _, key := range k.PrevSection.Keys() {
		if key == "h" || key == "left" {
			t.Fatalf("PrevSection still claims %q over the results", key)
		}
	}
	rows := modalRows(m)
	// The Confirm group (dimmed) still lists "h/l pick a button"; it is the
	// section row that must not claim h here.
	if !strings.Contains(rows, "Esc/h") || !strings.Contains(rows, "S-Tab/l") || strings.Contains(rows, "h/l           section") {
		t.Fatalf("results help should say Esc/h goes back and S-Tab goes to the previous section:\n%s", rows)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := updated.(dashboardModel); got.section != sectionOku {
		t.Fatalf("shift+tab over the results: section=%v, want the previous section", got.section)
	}

	// In Intro k goes to the previous section, so j/k cannot be labelled
	// "next section".
	intro := renderedDashboard(120, 40)
	intro.setSection(sectionIntro)
	updated, _ = intro.Update(runeKey('k'))
	if got := updated.(dashboardModel); got.section != sectionTimer {
		t.Fatalf("k in Intro: section=%v, want the previous section (Timer)", got.section)
	}
	updated, _ = intro.Update(runeKey('j'))
	if got := updated.(dashboardModel); got.section != sectionReading {
		t.Fatalf("j in Intro: section=%v, want the next section (Reading)", got.section)
	}
	if rows := modalRows(intro); strings.Contains(rows, "j/k           next section") || !strings.Contains(rows, "j/k           section") {
		t.Fatalf("Intro help should label j/k as moving between sections, not one way:\n%s", rows)
	}
	if d := intro.activeKeys().upDownDesc(); d != "section" {
		t.Fatalf("upDownDesc in Intro = %q, want section", d)
	}
}

func TestLaterToastDropsThePendingUndo(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setSection(sectionReading)
	m.showUndoToast("Moved 'Dune' to Read", undoAction{op: opStatus, bookID: 1,
		fromStatus: model.StatusRead, toStatus: model.StatusCurrentlyReading})
	if m.undo == nil {
		t.Fatal("the undo should be on offer")
	}

	// Something else fails before U is pressed: the toast that named the
	// undo is gone, and so is the undo.
	updated, _ := m.Update(opDoneMsg{op: opSync, err: errors.New("offline")})
	got := updated.(dashboardModel)
	if got.undo != nil {
		t.Fatal("a later toast must drop the pending undo")
	}
	if got.activeKeys().Undo.Enabled() {
		t.Fatal("U should not be advertised once its toast is gone")
	}
	before := got.inflight
	updated, cmd := got.Update(runeKey('U'))
	if cmd != nil || updated.(dashboardModel).inflight != before {
		t.Fatal("U after a later toast must do nothing")
	}
}

func TestToastExpiresOnItsOwnTickOnly(t *testing.T) {
	m := renderedDashboard(120, 40)

	first := m.showToast(toastInfo, "first")
	if first == nil {
		t.Fatal("a toast should arm its expiry tick")
	}
	if !strings.Contains(stripANSI(m.frame()), "first") {
		t.Fatal("the toast should show in the status bar")
	}
	firstSeq := m.toast.seq

	m.showToast(toastError, "second")
	if m.toast.text != "second" || m.toast.seq == firstSeq {
		t.Fatalf("toast = %+v, want the second one with a new seq", m.toast)
	}

	// The first toast's tick arrives late: the second one must survive it.
	updated, _ := m.Update(toastExpiredMsg{seq: firstSeq})
	got := updated.(dashboardModel)
	if got.toast.text != "second" {
		t.Fatalf("toast = %+v, want the newer toast left alone by the old tick", got.toast)
	}

	updated, _ = got.Update(toastExpiredMsg{seq: got.toast.seq})
	got = updated.(dashboardModel)
	if got.toast.text != "" {
		t.Fatalf("toast = %+v, want it cleared by its own tick", got.toast)
	}
	if strings.Contains(stripANSI(got.frame()), "second") {
		t.Fatal("an expired toast should leave the status bar")
	}

	// Errors get longer than notes, and the tick is a real Bubble Tea tick.
	if toastErrorTTL <= toastTTL {
		t.Fatalf("error TTL %v should outlast the info TTL %v", toastErrorTTL, toastTTL)
	}
}

func TestStatusChangeOffersUndoWhileTheToastIsUp(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setSection(sectionReading)
	m.inflight = 1

	updated, _ := m.Update(opDoneMsg{
		op: opStatus, info: "Status changed to Read", reload: true, markDirty: true,
		bookID: 1, title: "Dune", prevStatus: model.StatusCurrentlyReading, newStatus: model.StatusRead,
	})
	got := updated.(dashboardModel)

	if got.undo == nil || got.undo.op != opStatus || got.undo.bookID != 1 ||
		got.undo.toStatus != model.StatusCurrentlyReading || got.undo.fromStatus != model.StatusRead {
		t.Fatalf("undo = %+v, want the way back to Currently Reading for Dune", got.undo)
	}
	bar := stripANSI(got.statusBar())
	if !strings.Contains(bar, "Moved 'Dune' to Read") || !strings.Contains(bar, "U undo") {
		t.Fatalf("status bar = %q, want the move and the undo hint", bar)
	}
	if !got.activeKeys().Undo.Enabled() {
		t.Fatal("U should be live while the undo is on offer")
	}

	// The reload the result started is still in flight; undo must not wait
	// for it, the status it sets is absolute.
	before := got.inflight
	updated, cmd := got.Update(runeKey('U'))
	undone := updated.(dashboardModel)
	if cmd == nil || undone.inflight != before+1 {
		t.Fatalf("U should start the reverse status change (inflight %d → %d)", before, undone.inflight)
	}
	if undone.undo != nil {
		t.Fatal("an undo can only be taken once")
	}

	// Once the toast has expired, U does nothing.
	expired, _ := got.Update(toastExpiredMsg{seq: got.toast.seq})
	late, cmd := expired.(dashboardModel).Update(runeKey('U'))
	if cmd != nil || late.(dashboardModel).inflight != before {
		t.Fatal("U after the toast expired must not change anything")
	}
	if expired.(dashboardModel).activeKeys().Undo.Enabled() {
		t.Fatal("U should not be advertised once the undo is gone")
	}
}

func TestQuickProgressOffersUndoToThePreviousPage(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setSection(sectionReading)
	m.inflight = 1

	updated, _ := m.Update(opDoneMsg{
		op: opProgress, info: "Progress +10 → page 130", reload: true, markDirty: true,
		bookID: 1, title: "Dune", prevPage: 120, newPage: 130,
	})
	got := updated.(dashboardModel)

	if got.undo == nil || got.undo.op != opProgress || got.undo.toPage != 120 || got.undo.fromPage != 130 {
		t.Fatalf("undo = %+v, want the way back to page 120", got.undo)
	}
	if bar := stripANSI(got.statusBar()); !strings.Contains(bar, "Page 130") || !strings.Contains(bar, "U undo") {
		t.Fatalf("status bar = %q, want the page and the undo hint", bar)
	}

	before := got.inflight
	updated, cmd := got.Update(runeKey('U'))
	if cmd == nil || updated.(dashboardModel).inflight != before+1 {
		t.Fatal("U should start the update back to the previous page")
	}

	// A failed operation and one that changed nothing offer no undo.
	failed, _ := m.Update(opDoneMsg{op: opProgress, err: errors.New("offline"), bookID: 1, prevPage: 120, newPage: 130})
	if f := failed.(dashboardModel); f.undo != nil || f.toast.level != toastError {
		t.Fatalf("a failed update offered undo or hid the error: %+v", f.toast)
	}
	same, _ := m.Update(opDoneMsg{op: opProgress, info: "Progress updated to page 120", bookID: 1, prevPage: 120, newPage: 120})
	if same.(dashboardModel).undo != nil {
		t.Fatal("an update that changed nothing has nothing to undo")
	}
}

func TestUndoNeverStealsALetterFromTheSearchInput(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.showUndoToast("Moved 'Dune' to Read", undoAction{op: opStatus, bookID: 1,
		fromStatus: model.StatusRead, toStatus: model.StatusCurrentlyReading})
	m.setSection(sectionSearch)
	m.searchSub = searchSubInput
	m.enterSearchInsertMode()

	updated, _ := m.Update(runeKey('U'))
	got := updated.(dashboardModel)
	if got.searchInput.Value() != "U" {
		t.Fatalf("search input = %q, want the letter typed", got.searchInput.Value())
	}
	if got.undo == nil {
		t.Fatal("typing must not spend the undo")
	}
}

func TestFocusIsVisibleWithoutColour(t *testing.T) {
	m := renderedDashboard(120, 40)
	def := sectionDef{sectionReading, "Reading", 2}

	focused := ansi.Strip(m.renderSectionCard(def, 40, 8, true))
	blurred := ansi.Strip(m.renderSectionCard(def, 40, 8, false))
	if focused == blurred {
		t.Fatal("a focused and an unfocused card render the same once colour is stripped")
	}
	if !strings.Contains(focused, "▸") || strings.Contains(blurred, "▸") {
		t.Fatalf("the marker should follow the focus:\n%s\n%s", focused, blurred)
	}
	if !strings.Contains(focused, "┃") || strings.Contains(blurred, "┃") {
		t.Fatalf("the thick border should follow the focus:\n%s\n%s", focused, blurred)
	}

	// The right pane takes the focus border when the cursor moves into it.
	m.setSection(sectionSearch)
	m.searchSub = searchSubInput
	inputFrame := ansi.Strip(m.frame())
	m.searchSub = searchSubResults
	resultsFrame := ansi.Strip(m.frame())
	if inputFrame == resultsFrame {
		t.Fatal("focusing the search results should change the frame without colour")
	}
	if !m.rightPaneFocused() {
		t.Fatal("the search results live in the right pane")
	}
	// Two thick verticals per row: the focused card and the focused pane
	// never coexist, so the marked card is the only other thick border.
	if strings.Count(strings.Split(resultsFrame, "\n")[3], "┃") < 2 {
		t.Fatalf("the right pane should carry the thick border:\n%s", resultsFrame)
	}
}
