package tui

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
)

// setLibrary puts books on the shelves the way a load would and rebuilds the
// lists from them.
func setLibrary(m *Model, reading, oku []model.UserBook) {
	m.shared.reading, m.shared.oku = reading, oku
	m.broadcast(dataChangedMsg{dataLibrary})
}

func readingSection(m *Model) *librarySection {
	return m.sections[sectionReading].(*librarySection)
}

func okuSection(m *Model) *librarySection {
	return m.sections[sectionOku].(*librarySection)
}

// openReview puts the review modal up for book and returns it.
func openReview(m *Model, book model.UserBook) *reviewModal {
	m.push(newReviewModal(m.shared, m.st, book))
	return m.topModal().(*reviewModal)
}

func TestSearchInputNormalModeNavigation(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionSearch)
	m.search.sub = searchSubInput
	m.search.mode = searchModeNormal
	m.search.input.SetValue("dune")

	// Press 'k' (up) → should go to previous section (sectionOku).
	send(t, m, runeKey('k'))

	if m.tab != sectionOku {
		t.Fatalf("section after k = %v, want %v", m.tab, sectionOku)
	}
	if m.search.input.Value() != "dune" {
		t.Fatalf("search input changed in normal mode, got %q", m.search.input.Value())
	}
	if m.search.mode != searchModeNormal {
		t.Fatalf("search mode = %v, want normal", m.search.mode)
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
	m := newTestModel()
	_, cmd := m.Update(libraryLoadedMsg{
		reading:      []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}},
		needsRefresh: true,
	})

	if !m.shared.loaded {
		t.Fatal("cached library should mark dashboard loaded")
	}
	if !m.isLoading() {
		t.Fatal("stale cache should start a background refresh")
	}
	if len(m.shared.reading) != 1 || m.shared.reading[0].Book.Title != "Dune" {
		t.Fatalf("cached reading books = %#v, want Dune", m.shared.reading)
	}
	if cmd == nil {
		t.Fatal("stale cache should return a refresh command")
	}
}

func TestSearchInputInsertModeTypingAndEsc(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionSearch)
	m.search.sub = searchSubInput
	m.search.enterInsertMode()

	m.Update(runeKey('h'))

	if m.tab != sectionSearch {
		t.Fatalf("section after typing in insert mode = %v, want %v", m.tab, sectionSearch)
	}
	if m.search.input.Value() != "h" {
		t.Fatalf("search input value = %q, want %q", m.search.input.Value(), "h")
	}
	if m.search.mode != searchModeInsert {
		t.Fatalf("search mode after typing = %v, want insert", m.search.mode)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.search.mode != searchModeNormal {
		t.Fatalf("search mode after esc = %v, want normal", m.search.mode)
	}
	if m.tab != sectionSearch {
		t.Fatalf("section after esc = %v, want %v", m.tab, sectionSearch)
	}
}

func TestLibrarySectionVimNavigation(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionReading)

	m.Update(runeKey('l'))
	if m.tab != sectionOku {
		t.Fatalf("section after l = %v, want %v", m.tab, sectionOku)
	}

	m.Update(runeKey('h'))
	if m.tab != sectionReading {
		t.Fatalf("section after h = %v, want %v", m.tab, sectionReading)
	}
}

func TestSearchInputNormalModeEscGoesBack(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionSearch)
	m.search.sub = searchSubInput
	m.search.mode = searchModeNormal

	send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.tab != sectionOku {
		t.Fatalf("section after Esc = %v, want %v", m.tab, sectionOku)
	}
}

func TestSubmitSearchSetsLoadingState(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionSearch)
	m.search.input.SetValue("dune")
	m.search.queryMode = model.SearchModeAuthor

	cmd := send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submitting a non-empty query should start a search")
	}
	if !m.isLoading() || !m.search.loading {
		t.Fatalf("loading flags = loading:%v searchLoading:%v, want true/true", m.isLoading(), m.search.loading)
	}

	if m.search.loadingQuery != "dune" {
		t.Fatalf("loadingQuery = %q, want dune", m.search.loadingQuery)
	}
	if !strings.Contains(m.search.list.Title, "loading") {
		t.Fatalf("search list title %q does not include loading state", m.search.list.Title)
	}
	if !strings.Contains(strings.ToLower(m.toast.text), "searching for") {
		t.Fatalf("toast %q does not include searching feedback", m.toast.text)
	}
}

func TestSubmitSearchGuardAndEmptyValidation(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionSearch)
	m.search.loading = true
	m.search.input.SetValue("dune")
	if cmd := send(t, m, tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Fatal("a query typed over an in-flight search should be searchable")
	}

	m.search.input.SetValue("   ")
	m.inflight = 0
	m.search.loading = false

	send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.isLoading() || m.search.loading {
		t.Fatal("an empty query should not be searched for")
	}
	if m.toast.level != toastError || m.toast.text == "" {
		t.Fatalf("toast = %+v, want a validation error for the empty query", m.toast)
	}
}

func TestSearchLoadedMsgTransitionsToResults(t *testing.T) {
	m := newTestModel()
	m.inflight = 1
	m.setSection(sectionSearch)
	m.search.loading = true
	m.search.sub = searchSubInput
	m.search.queryMode = model.SearchModeAuthor

	send(t, m, searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}},
		query:   "dune",
		mode:    model.SearchModeAuthor,
	})

	if m.isLoading() || m.search.loading {
		t.Fatalf("loading flags after response = loading:%v searchLoading:%v, want false/false", m.isLoading(), m.search.loading)
	}

	if m.search.sub != searchSubResults {
		t.Fatalf("searchSub = %v, want %v", m.search.sub, searchSubResults)
	}
	if m.search.lastQuery != "dune" {
		t.Fatalf("lastQuery = %q, want dune", m.search.lastQuery)
	}
	if m.search.lastMode != model.SearchModeAuthor {
		t.Fatalf("lastMode = %q, want %q", m.search.lastMode, model.SearchModeAuthor)
	}

	if m.search.list.Title != "AUTHOR Results (1)" {
		t.Fatalf("searchList title = %q, want %q", m.search.list.Title, "AUTHOR Results (1)")
	}
	if !strings.Contains(m.toast.text, "loaded 1 results") {
		t.Fatalf("toast = %q, expected loaded-count feedback", m.toast.text)
	}
}

func TestSlashSearchPreservesExistingQuery(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionReading)
	m.search.input.SetValue("dune")

	m.Update(runeKey('/'))

	if m.tab != sectionSearch {
		t.Fatalf("section after / = %v, want %v", m.tab, sectionSearch)
	}
	if m.search.sub != searchSubInput {
		t.Fatalf("searchSub after / = %v, want %v", m.search.sub, searchSubInput)
	}
	if m.search.input.Value() != "dune" {
		t.Fatalf("search query after / = %q, want %q", m.search.input.Value(), "dune")
	}
}

func TestTimerStartOpensBookSelectionFirst(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionTimer)
	setLibrary(m, []model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune"}},
		{Book: model.Book{ID: 2, Title: "Foundation"}},
	}, nil)

	send(t, m, runeKey('t'))

	if m.isLoading() {
		t.Fatal("timer start should open selection first, got an immediate operation")
	}
	if m.timerPicker() == nil {
		t.Fatal("the picker should be up after pressing t")
	}
}

func TestSearchResultsKStaysInResults(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionSearch)
	m.search.sub = searchSubResults
	m.search.mode = searchModeNormal
	m.search.list.SetItems([]list.Item{
		searchResultItem{result: model.SearchResult{ID: 1, Title: "Dune"}},
		searchResultItem{result: model.SearchResult{ID: 2, Title: "Dune Messiah"}},
	})
	m.search.list.Select(1)

	m.Update(runeKey('k'))

	if m.search.sub != searchSubResults {
		t.Fatalf("searchSub after k = %v, want %v", m.search.sub, searchSubResults)
	}
	if m.tab != sectionSearch {
		t.Fatalf("section after k = %v, want %v", m.tab, sectionSearch)
	}
}

func TestCycleDensityRefreshesSearchResultItems(t *testing.T) {
	m := newTestModel()
	m.shared.density = DensityDefault
	m.search.results = []model.SearchResult{
		{ID: 1, Title: "Dune", Authors: []string{"Frank Herbert"}, Slug: "dune"},
	}
	m.search.rebuildResults()

	send(t, m, runeKey('z'))

	items := m.search.list.Items()
	if len(items) != 1 {
		t.Fatalf("search items len = %d, want 1", len(items))
	}
	item, ok := items[0].(searchResultItem)
	if !ok {
		t.Fatalf("item type = %T, want searchResultItem", items[0])
	}
	if item.density != DensityVerbose {
		t.Fatalf("search item density = %v, want %v", item.density, DensityVerbose)
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
			got, err := model.ParseRating(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("model.ParseRating(%q) expected error", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("model.ParseRating(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("model.ParseRating(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestReviewSaveKeepsModalOpenWhileSaving(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	review := openReview(m, model.UserBook{Book: model.Book{ID: 42, Title: "Dune"}})
	review.rating.SetValue("3")
	review.text.SetValue("Strong first half.")

	cmd := send(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})

	if cmd == nil {
		t.Fatal("expected save command")
	}
	if m.topModal() != review {
		t.Fatal("review modal should stay open until the save succeeds")
	}
	if !review.submitting {
		t.Fatal("submitting should be true while the save is in flight")
	}
	if !m.isLoading() {
		t.Fatal("loading should be true while save is in flight")
	}
	if m.toast.text != "Saving review..." {
		t.Fatalf("toast = %q, want Saving review...", m.toast.text)
	}
}

func TestTimerSelectEnterClampsStaleIndex(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionTimer)
	setLibrary(m, []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}}, nil)
	// Stale: the list shrank while the picker was open.
	m.push(newTimerPickerModal(m.shared, 5))

	cmd := send(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("enter on a valid list should start the timer")
	}
	if m.timerPicker() != nil {
		t.Fatal("the picker should close after enter")
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
	book := model.UserBook{Book: model.Book{ID: 7, Title: "Dune"}}
	tests := []struct {
		name string
		open func(m *Model)
	}{
		{name: "library", open: func(*Model) {}},
		{name: "page modal", open: func(m *Model) {
			m.push(newPageModal(m.shared, m.st, book))
			m.pagePrompt().input.SetValue("120")
		}},
		{name: "review modal", open: func(m *Model) { openReview(m, book) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			m.shared.loaded = true
			m.setSection(sectionSearch)
			m.search.sub = searchSubInput
			m.search.enterInsertMode()
			m.search.input.SetValue("dune")
			if cmd := send(t, m, tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
				t.Fatal("submitting the query should start a search")
			}

			tt.open(m)
			top := m.topModal()

			send(t, m, searchLoadedMsg{
				results: []model.SearchResult{{ID: 1, Title: "Dune"}},
				query:   "dune",
				mode:    model.SearchModeBook,
				seq:     m.search.seq,
			})

			if m.search.loading {
				t.Fatal("searchLoading should be cleared by the result, whatever the mode")
			}
			if m.isLoading() {
				t.Fatal("loading should be cleared by the result, whatever the mode")
			}
			if len(m.search.results) != 1 {
				t.Fatalf("results = %d, want 1", len(m.search.results))
			}
			if m.topModal() != top {
				t.Fatalf("top modal = %T, want %T: an async result must not close a modal", m.topModal(), top)
			}
			if m.search.sub != searchSubResults {
				t.Fatalf("searchSub = %v, want %v", m.search.sub, searchSubResults)
			}
			if m.search.mode != searchModeNormal {
				t.Fatal("results should leave search insert mode so j/k scroll them")
			}
			if page := m.pagePrompt(); page != nil && page.input.Value() != "120" {
				t.Fatalf("page input = %q, want it untouched", page.input.Value())
			}
		})
	}
}

func TestStaleSearchResultIsIgnored(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionSearch)
	m.search.input.SetValue("dune")
	if cmd := send(t, m, tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Fatal("the first query should start a search")
	}
	staleSeq := m.search.seq

	// The user retypes before the first response lands.
	m.search.loading = false
	m.search.input.SetValue("foundation")
	if cmd := send(t, m, tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Fatal("the second query should start a search")
	}

	_, cmd := m.Update(searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}},
		query:   "dune",
		mode:    model.SearchModeBook,
		seq:     staleSeq,
	})

	if cmd != nil {
		t.Fatal("a superseded search result should produce no command")
	}
	if !m.search.loading {
		t.Fatal("a superseded result must not clear the in-flight search")
	}
	if len(m.search.results) != 0 {
		t.Fatalf("results = %d, want 0 for a superseded result", len(m.search.results))
	}

	send(t, m, searchLoadedMsg{
		results: []model.SearchResult{{ID: 2, Title: "Foundation"}},
		query:   "foundation",
		mode:    model.SearchModeBook,
		seq:     m.search.seq,
	})

	if m.search.loading {
		t.Fatal("the latest result should clear searchLoading")
	}
	if m.search.lastQuery != "foundation" {
		t.Fatalf("lastQuery = %q, want foundation", m.search.lastQuery)
	}
}

func TestSearchResultKeepsUserSelectedMode(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionSearch)
	m.search.queryMode = model.SearchModeGenre

	send(t, m, searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}},
		query:   "dune",
		mode:    model.SearchModeBook,
		seq:     m.search.seq,
	})

	if m.search.queryMode != model.SearchModeGenre {
		t.Fatalf("queryMode = %q, want the mode the user picked (%q)", m.search.queryMode, model.SearchModeGenre)
	}
	if m.search.list.Title != "BOOK Results (1)" {
		t.Fatalf("searchList title = %q, want the mode the results came from", m.search.list.Title)
	}
}

func TestTimerOpDoneReleasesItsSlot(t *testing.T) {
	m := newTestModel()
	m.inflight = 1
	m.push(newTimerPickerModal(m.shared, 0))

	_, cmd := m.Update(timerOpDoneMsg{info: "Timer started — Dune"})

	// The timer operation's slot is released; only the local-data reload it
	// starts is left in flight.
	if m.inflight != 1 {
		t.Fatalf("inflight = %d, want 1 (the reload): the timer op must release its slot", m.inflight)
	}
	if m.timerPicker() != nil {
		t.Fatal("timerOpDoneMsg should close the timer picker")
	}
	if m.toast.text != "Timer started — Dune" {
		t.Fatalf("toast = %q, want the timer info", m.toast.text)
	}
	if cmd == nil {
		t.Fatal("timer operations should reload local data")
	}

	m.Update(localDataLoadedMsg{})
	if m.isLoading() {
		t.Fatal("loading should clear once the reload lands")
	}
}

func TestLocalDataClearsLoadingAndArmsTimerTick(t *testing.T) {
	m := newTestModel()
	m.inflight = 1
	m.timerTicking = true

	// With no timer running the one-second tick loop stops.
	_, cmd := m.Update(timerTickMsg(time.Now()))
	if cmd != nil {
		t.Fatal("the timer tick should not re-arm while no timer runs")
	}
	if m.timerTicking {
		t.Fatal("timerTicking should be cleared when no timer runs")
	}

	_, cmd = m.Update(localDataLoadedMsg{
		timerState: &model.TimerState{BookID: 1, StartedAt: time.Now()},
		timerBook:  &model.Book{ID: 1, Title: "Dune"},
	})

	if m.isLoading() {
		t.Fatal("localDataLoadedMsg should clear loading")
	}
	if cmd == nil || !m.timerTicking {
		t.Fatal("a running timer should arm the one-second tick")
	}
	if m.shared.timerBook == nil || m.shared.timerBook.Title != "Dune" {
		t.Fatal("the running timer's book should be resolved into shared state")
	}
}

func TestSpinnerStopsWhenNothingIsInFlight(t *testing.T) {
	m := newTestModel()
	m.spinning = true

	_, cmd := m.Update(spinner.TickMsg{})

	if cmd != nil {
		t.Fatal("an idle dashboard should stop re-arming the spinner")
	}
	if m.spinning {
		t.Fatal("spinning should be cleared once nothing is in flight")
	}
	if m.beginLoading(loadLocalDataCmd(m.app, m.shared.now)) == nil {
		t.Fatal("starting new work should re-arm the spinner")
	}
	if cmd := m.beginLoading(loadLocalDataCmd(m.app, m.shared.now)); cmd == nil {
		t.Fatal("the second operation should still be batched")
	} else if !m.spinning || m.inflight != 2 {
		t.Fatalf("inflight = %d, spinning = %v: a second operation must be counted without a second tick loop", m.inflight, m.spinning)
	}
}

func TestOverlappingLoadsKeepLoadingUntilBothFinish(t *testing.T) {
	m := newTestModel()
	if cmd := m.beginLoading(loadLocalDataCmd(m.app, m.shared.now), loadLibraryCmd(m.ctx, m.app, false)); cmd == nil {
		t.Fatal("beginLoading() returned no command")
	}
	if !m.isLoading() {
		t.Fatal("two commands are in flight")
	}

	m.Update(localDataLoadedMsg{})
	if !m.isLoading() {
		t.Fatal("the first result must not clear loading while the second load runs")
	}

	m.Update(libraryLoadedMsg{})
	if m.isLoading() {
		t.Fatal("loading should clear once every command has reported")
	}
}

func TestQuickProgressIsGuardedWhileInFlight(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.setSection(sectionReading)
	setLibrary(m, []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}}, nil)

	cmd := send(t, m, runeKey('+'))
	if cmd == nil {
		t.Fatal("the first + should start a progress update")
	}
	if !m.isLoading() {
		t.Fatal("the first + should mark the update in flight")
	}

	send(t, m, runeKey('+'))
	if m.inflight != 1 {
		t.Fatalf("inflight = %d: a second + while an update is in flight must not fire, it would lose one", m.inflight)
	}
	if m.toast.text == "" || m.toast.level != toastWarn {
		t.Fatalf("toast = %+v, want the refused update to say why", m.toast)
	}
}

func TestReviewSaveFailureKeepsModalOpen(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	review := openReview(m, model.UserBook{Book: model.Book{ID: 42, Title: "Dune"}})
	review.rating.SetValue("3")
	review.text.SetValue("Strong first half.")

	send(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m.Update(opDoneMsg{op: opReview, seq: review.token, err: errors.New("save failed")})

	if m.isLoading() {
		t.Fatal("a failed save should neither stay in flight nor trigger a reload")
	}
	if m.topModal() != review {
		t.Fatal("a failed save must keep the modal open")
	}
	if review.submitting {
		t.Fatal("submitting should be cleared once the save failed")
	}
	if review.text.Value() != "Strong first half." {
		t.Fatalf("review text = %q, want the draft to survive", review.text.Value())
	}
	if review.err != "save failed" {
		t.Fatalf("err = %q, want save failed", review.err)
	}

	if !strings.Contains(review.View(m.lay, m.st), "save failed") {
		t.Fatal("the failure should be visible inside the overlay, not behind it")
	}
}

func TestUnrelatedOpDoesNotDisturbReviewModal(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	review := openReview(m, model.UserBook{Book: model.Book{ID: 42, Title: "Dune"}})
	review.text.SetValue("half written")

	m.Update(opDoneMsg{
		op:        opProgress,
		info:      "Progress +10 → page 40",
		reload:    true,
		markDirty: true,
	})

	if m.topModal() != review {
		t.Fatal("another operation's result must not close the review modal")
	}
	if review.text.Value() != "half written" {
		t.Fatalf("review text = %q, want the draft untouched", review.text.Value())
	}
	if !m.dirty {
		t.Fatal("the unrelated mutation should still mark the library dirty")
	}
	if m.toast.text != "Progress +10 → page 40" {
		t.Fatalf("toast = %q, want the unrelated result reported", m.toast.text)
	}
}

func TestReviewSaveSuccessClosesModal(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	review := openReview(m, model.UserBook{Book: model.Book{ID: 42, Title: "Dune"}})
	review.rating.SetValue("3")

	send(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	_, cmd := m.Update(opDoneMsg{
		op:        opReview,
		seq:       review.token,
		info:      "Updated rating (★★★)",
		reload:    true,
		markDirty: true,
	})

	if m.topModal() != nil {
		t.Fatal("a successful save should close the modal")
	}
	if cmd == nil {
		t.Fatal("a successful save should reload the library")
	}
	if !m.dirty {
		t.Fatal("a successful save should mark the library dirty")
	}
}

func TestBackgroundReconcileClearsDirtyOnlyOnSuccess(t *testing.T) {
	overdue := func() *Model {
		m := newTestModel()
		m.shared.loaded = true
		m.dirty = true
		m.lastMutationAt = time.Now().Add(-2 * backgroundSyncWindow)
		return m
	}

	started := overdue()
	_, cmd := started.Update(backgroundCheckMsg{})
	if cmd == nil {
		t.Fatal("an overdue reconcile should start a refresh")
	}
	if !started.dirty {
		t.Fatal("dirty must stay set until the reconcile actually succeeds")
	}
	if !started.reconciling {
		t.Fatal("reconciling should mark the in-flight reconcile")
	}

	failed := overdue()
	failed.Update(backgroundCheckMsg{})
	failed.Update(libraryLoadedMsg{reconcile: true, err: errors.New("offline")})
	if !failed.dirty {
		t.Fatal("a failed reconcile must leave the library dirty")
	}
	if failed.reconciling {
		t.Fatal("a failed reconcile should release the reconcile slot so it can retry")
	}

	started.Update(libraryLoadedMsg{reconcile: true})
	if started.dirty {
		t.Fatal("a successful reconcile should clear dirty")
	}
}

func TestUnrelatedLibraryLoadDoesNotClearDirty(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.dirty = true
	m.lastMutationAt = time.Now().Add(-2 * backgroundSyncWindow)

	m.Update(backgroundCheckMsg{})
	if !m.reconciling {
		t.Fatal("the reconcile should be in flight")
	}

	// A status change lands while the reconcile is still running and triggers
	// its own cache reload, whose result arrives first.
	m.Update(opDoneMsg{op: opStatus, info: "Status changed", reload: true, markDirty: true})
	m.Update(libraryLoadedMsg{})

	if !m.dirty {
		t.Fatal("an unrelated library load must not clear dirty")
	}
	if !m.reconciling {
		t.Fatal("the reconcile is still in flight")
	}

	m.Update(libraryLoadedMsg{reconcile: true})
	if m.dirty || m.reconciling {
		t.Fatalf("the reconcile result should clear dirty: dirty=%v reconciling=%v", m.dirty, m.reconciling)
	}
}

func TestPendingProgressResultDoesNotClosePageModal(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.setSection(sectionReading)
	setLibrary(m, []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}}, nil)

	send(t, m, runeKey('+')) // progress update in flight
	send(t, m, runeKey('u')) // user opens the page modal
	if m.pagePrompt() == nil {
		t.Fatalf("top modal = %T, want the page prompt open", m.topModal())
	}

	m.Update(opDoneMsg{op: opProgress, info: "Progress +10 → page 40", markDirty: true})

	if m.pagePrompt() == nil {
		t.Fatal("a progress result started before the modal opened must not close it")
	}
}

func TestPageModalEnterIsGuardedWhileInFlight(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.setSection(sectionReading)
	setLibrary(m, []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}}, nil)

	send(t, m, runeKey('+'))
	send(t, m, runeKey('u'))
	m.pagePrompt().input.SetValue("120")

	send(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.inflight != 1 {
		t.Fatalf("inflight = %d: enter must not submit a page update while one is in flight", m.inflight)
	}
	if m.pagePrompt() == nil {
		t.Fatal("the modal should stay open after a refused submission")
	}
	if m.toast.text == "" {
		t.Fatal("the refused submission should say why")
	}
}

func TestPageModalClosesOnItsOwnResult(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.push(newPageModal(m.shared, m.st, model.UserBook{Book: model.Book{ID: 1, Title: "Dune"}}))
	page := m.pagePrompt()
	page.input.SetValue("120")

	cmd := send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil || !m.isLoading() {
		t.Fatal("enter should submit the page update")
	}

	m.Update(opDoneMsg{op: opProgress, seq: page.token, info: "Progress updated to page 120", markDirty: true})

	if m.pagePrompt() != nil {
		t.Fatal("the modal's own result should close it")
	}
}

func TestModalStackPopsOnOwnResultOnly(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.push(newPageModal(m.shared, m.st, model.UserBook{Book: model.Book{ID: 1, Title: "Dune"}}))
	page := m.pagePrompt()

	// A progress result from elsewhere, and another operation stamped with
	// the same token, are not the prompt's own.
	m.Update(opDoneMsg{op: opProgress, seq: page.token + 1, info: "Progress +10 → page 40"})
	if m.pagePrompt() == nil {
		t.Fatal("another session's progress result must not close the prompt")
	}
	m.Update(opDoneMsg{op: opReview, seq: page.token, info: "Updated rating"})
	if m.pagePrompt() == nil {
		t.Fatal("a different operation with the same token must not close the prompt")
	}

	m.Update(opDoneMsg{op: opProgress, seq: page.token, info: "Progress updated to page 120"})
	if m.topModal() != nil {
		t.Fatalf("top modal = %T, want the prompt closed by its own result", m.topModal())
	}

	// A modal under the top sees its result too: the picker under help
	// closes when the timer starts, and help stays.
	m.push(newTimerPickerModal(m.shared, 0))
	m.openHelp()
	m.Update(timerOpDoneMsg{info: "Timer started"})
	if len(m.modals) != 1 {
		t.Fatalf("modals = %d, want the picker gone and help kept", len(m.modals))
	}
	if _, ok := m.topModal().(*helpModal); !ok {
		t.Fatalf("top modal = %T, want help", m.topModal())
	}
}

func TestLibraryReloadRunsTheSetItemsCmd(t *testing.T) {
	m := renderedDashboard(120, 40)
	reading := readingSection(m)
	// An active filter: SetItems must re-run it, or the list goes blank.
	reading.list.SetFilterText("dune")
	if got := reading.list.VisibleItems(); len(got) != 1 {
		t.Fatalf("filtered list shows %d items, want 1", len(got))
	}

	_, cmd := m.Update(libraryLoadedMsg{reading: []model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune", Pages: 412}, CurrentPage: 130},
		{Book: model.Book{ID: 9, Title: "Ubik"}},
	}})

	var matches *list.FilterMatchesMsg
	var walk func(cmd tea.Cmd)
	walk = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				walk(c)
			}
		case list.FilterMatchesMsg:
			matches = &msg
		}
	}
	walk(cmd)
	if matches == nil {
		t.Fatal("the reload must return the list's filter command, or an active filter goes blank")
	}

	m.Update(*matches)
	got := reading.list.VisibleItems()
	if len(got) != 1 || got[0].(userBookItem).book.Book.Title != "Dune" {
		t.Fatalf("filtered list after the reload = %#v, want Dune", got)
	}
}

func TestSearchInsertModeTypesQuestionMark(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionSearch)
	m.search.sub = searchSubInput
	m.search.enterInsertMode()

	m.Update(runeKey('?'))

	if m.topModal() != nil {
		t.Fatal("? should be typed in insert mode, not open help")
	}
	if m.search.input.Value() != "?" {
		t.Fatalf("search input = %q, want ?", m.search.input.Value())
	}
}

func TestSectionFocusResizesLists(t *testing.T) {
	m := newTestModel()
	m.lay = layout{W: 100, H: 44}
	m.setSection(sectionReading)

	readingFocused := readingSection(m).list.Height()

	m.Update(runeKey('l')) // Oku takes the focus, and with it the extra rows.
	if m.tab != sectionOku {
		t.Fatalf("section = %v, want %v", m.tab, sectionOku)
	}
	if readingSection(m).list.Height() >= readingFocused {
		t.Fatalf("reading list height = %d, want less than the focused %d", readingSection(m).list.Height(), readingFocused)
	}

	heights := m.leftSectionHeights(m.rightPanelContentHeight())
	if want := max(1, heights[sectionReading]-3); readingSection(m).list.Height() != want {
		t.Fatalf("reading list height = %d, want %d", readingSection(m).list.Height(), want)
	}
	if want := max(1, heights[sectionOku]-3); okuSection(m).list.Height() != want {
		t.Fatalf("oku list height = %d, want %d", okuSection(m).list.Height(), want)
	}
}

func TestSlashFocusResizesLists(t *testing.T) {
	m := newTestModel()
	m.lay = layout{W: 100, H: 44}
	m.setSection(sectionReading)

	m.Update(runeKey('/'))

	heights := m.leftSectionHeights(m.rightPanelContentHeight())
	if want := max(1, heights[sectionReading]-3); readingSection(m).list.Height() != want {
		t.Fatalf("reading list height after / = %d, want %d", readingSection(m).list.Height(), want)
	}
}

func TestReviewModalIsReadOnlyWhileSaving(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	review := openReview(m, model.UserBook{Book: model.Book{ID: 42, Title: "Dune"}})
	review.rating.SetValue("3")
	review.text.SetValue("Strong first half.")

	send(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	pendingSeq := review.token

	m.Update(runeKey('x'))
	if review.text.Value() != "Strong first half." {
		t.Fatalf("review text = %q, want it read-only while saving", review.text.Value())
	}

	// Cancelling drops the pending result instead of reopening the modal.
	send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.topModal() != nil {
		t.Fatal("esc should close the modal even while saving")
	}

	m.Update(opDoneMsg{op: opReview, seq: pendingSeq, err: errors.New("save failed")})
	if m.topModal() != nil {
		t.Fatal("a result for a cancelled modal must not reopen it")
	}
	if m.toast.text != "save failed" || m.toast.level != toastError {
		t.Fatalf("toast = %+v, want the failure reported in the status bar", m.toast)
	}
}

func TestSearchErrorClearsLoadingTitle(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionSearch)
	m.search.input.SetValue("dune")
	if cmd := send(t, m, tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Fatal("submitting the query should start a search")
	}

	send(t, m, searchLoadedMsg{
		query: "dune",
		mode:  model.SearchModeBook,
		seq:   m.search.seq,
		err:   errors.New("network down"),
	})

	if strings.Contains(m.search.list.Title, "loading") {
		t.Fatalf("searchList title = %q, want the loading state cleared", m.search.list.Title)
	}
	if m.search.loading {
		t.Fatal("a failed search should clear searchLoading")
	}
}

// renderedDashboard builds a loaded dashboard of the given size with a couple
// of books on each shelf, ready to render.
func renderedDashboard(w, h int) *Model {
	m := newTestModel()
	m.shared.loaded = true
	setLibrary(m, []model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune", Pages: 412}, CurrentPage: 120},
		{Book: model.Book{ID: 2, Title: "Foundation", Pages: 255}},
	}, []model.UserBook{
		{Book: model.Book{ID: 3, Title: "Meditations"}},
	})
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
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
	m.search.enterInsertMode()

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
	m := newTestModel()
	if got := m.search.input.AvailableSuggestions(); len(got) != 0 {
		t.Fatalf("a fresh dashboard suggests %#v, want nothing until the user searches", got)
	}

	// A nil app must not panic on the way to the (skipped) save.
	if cmd := m.addRecentSearchQuery("east of eden"); cmd != nil {
		t.Fatal("addRecentSearchQuery() should not try to save without a store")
	}
	m.search.updateSuggestions()

	got := m.search.input.AvailableSuggestions()
	if len(got) != 1 || got[0] != "east of eden" {
		t.Fatalf("suggestions = %#v, want the query just searched for", got)
	}
}

func TestLocalDataMergesStoredRecentSearches(t *testing.T) {
	m := newTestModel()
	m.inflight = 1
	m.addRecentSearchQuery("dune")

	m.Update(localDataLoadedMsg{recentSearches: []string{"dune", "clean code"}})

	want := []string{"dune", "clean code"}
	if len(m.shared.recentSearches) != len(want) {
		t.Fatalf("recentSearches = %#v, want %#v", m.shared.recentSearches, want)
	}
	for i := range want {
		if m.shared.recentSearches[i] != want[i] {
			t.Fatalf("recentSearches[%d] = %q, want %q", i, m.shared.recentSearches[i], want[i])
		}
	}
}

func TestTimerStartsAndStopsFromReadingList(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setSection(sectionReading)

	cmd := send(t, m, runeKey('t'))
	if cmd == nil {
		t.Fatal("t in the Reading list should start a timer for the selection")
	}
	if !m.isLoading() {
		t.Fatal("starting a timer should be counted as in flight")
	}
	if m.timerPicker() != nil {
		t.Fatal("the Reading list already has a selection: no picker")
	}

	// With a timer running, t stops it.
	running := renderedDashboard(100, 40)
	running.setSection(sectionReading)
	running.shared.timer = &model.TimerState{BookID: 1, StartedAt: time.Now()}

	if cmd := send(t, running, runeKey('t')); cmd == nil {
		t.Fatal("t should stop the running timer")
	}
	if running.timerPicker() != nil {
		t.Fatal("stopping a timer must not open the picker")
	}
}

func TestTimerKeyOutsideReadingExplainsItself(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setSection(sectionOku)

	send(t, m, runeKey('t'))
	if m.isLoading() {
		t.Fatal("t in the Oku list has no book to track, so it should start nothing")
	}
	if !strings.Contains(m.toast.text, "Reading list") {
		t.Fatalf("toast = %q, want it to point at the Reading list", m.toast.text)
	}
}

func TestEnterDoesNotChangeStatus(t *testing.T) {
	for _, section := range []focusSection{sectionReading, sectionOku} {
		m := renderedDashboard(100, 40)
		m.setSection(section)

		send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.isLoading() {
			t.Fatalf("section %v: Enter started an operation", section)
		}
		if m.toast.level != toastInfo {
			t.Fatalf("section %v: toast = %+v, want no error", section, m.toast)
		}
		if m.toast.text == "" {
			t.Fatalf("section %v: Enter should name the book it brought into the detail pane", section)
		}
	}
}

func TestPageModalShowsTitleAndKeepsFormatHint(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setSection(sectionReading)

	send(t, m, runeKey('u'))

	page := m.pagePrompt()
	if page == nil {
		t.Fatalf("top modal after u = %T, want the page prompt", m.topModal())
	}
	if page.input.Value() != "" {
		t.Fatalf("page input pre-filled with %q, want it empty", page.input.Value())
	}
	if page.input.Placeholder != "370 or +10 or -5" {
		t.Fatalf("placeholder = %q, want the format hint", page.input.Placeholder)
	}

	prompt := page.View(m.lay, m.st)
	if !strings.Contains(prompt, "Dune") {
		t.Fatalf("prompt %q does not name the book", prompt)
	}
	if !strings.Contains(prompt, "current: 120/412") {
		t.Fatalf("prompt %q does not show where the book stands", prompt)
	}

	// The prompt is two rows taller than the help bar it replaces, and the
	// layout has to give those rows back.
	if lines := strings.Split(m.frame(), "\n"); len(lines) != 40 {
		t.Fatalf("page prompt frame has %d lines, want 40", len(lines))
	}
}

func TestHelpModalFitsTheTerminalAndScrolls(t *testing.T) {
	for _, h := range []int{24, 40} {
		m := renderedDashboard(80, h)

		m.Update(runeKey('?'))
		help, ok := m.topModal().(*helpModal)
		if !ok {
			t.Fatalf("height %d: ? should open the help modal, top is %T", h, m.topModal())
		}
		if lines := strings.Split(m.frame(), "\n"); len(lines) != h {
			t.Fatalf("height %d: help frame has %d lines, want %d", h, len(lines), h)
		}
		if help.vp.TotalLineCount() <= help.vp.Height {
			t.Fatalf("height %d: the help body should be taller than the window", h)
		}
		if !strings.Contains(help.View(m.lay, m.st), "j/k scroll") {
			t.Fatalf("height %d: an overflowing help modal should say it scrolls", h)
		}

		m.Update(runeKey('j'))
		if help.vp.YOffset != 1 {
			t.Fatalf("height %d: j should scroll the body, YOffset = %d", h, help.vp.YOffset)
		}

		m.Update(runeKey('k'))
		if got := help.vp.YOffset; got != 0 {
			t.Fatalf("height %d: k should scroll back, YOffset = %d", h, got)
		}

		m.Update(runeKey('?'))
		if m.topModal() != nil {
			t.Fatalf("height %d: ? should close the help modal", h)
		}
	}
}

func TestIgnoreAsksBeforeItChangesTheStatus(t *testing.T) {
	asked := func() *Model {
		m := renderedDashboard(100, 40)
		m.setSection(sectionReading)
		if cmd := send(t, m, runeKey('x')); cmd != nil {
			t.Fatal("x should ask before it changes the status")
		}
		return m
	}

	m := asked()
	confirm, ok := m.topModal().(*confirmModal)
	if !ok {
		t.Fatalf("top modal = %T, want the confirmation", m.topModal())
	}
	if !strings.Contains(confirm.c.Message, "Dune") {
		t.Fatalf("confirm message = %q, want it to name the book", confirm.c.Message)
	}
	if !strings.Contains(confirm.c.Message, model.StatusIgnored.Label()) {
		t.Fatalf("confirm message = %q, want it to name the new status", confirm.c.Message)
	}

	cancelled := asked()
	send(t, cancelled, runeKey('n'))
	if cancelled.topModal() != nil || cancelled.isLoading() {
		t.Fatal("n should drop the change without running anything")
	}

	confirmed := asked()
	if cmd := send(t, confirmed, runeKey('y')); cmd == nil {
		t.Fatal("y should run the status change")
	}
	if confirmed.topModal() != nil {
		t.Fatal("the confirmation should close once it is answered")
	}
	if !confirmed.isLoading() {
		t.Fatal("the confirmed change should be counted as in flight")
	}

	// Esc is the same answer as n.
	escaped := asked()
	send(t, escaped, tea.KeyMsg{Type: tea.KeyEsc})
	if escaped.topModal() != nil || escaped.isLoading() {
		t.Fatal("esc should drop the change")
	}
}

func TestDidNotFinishAsksToo(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setSection(sectionReading)

	cmd := send(t, m, runeKey('d'))
	confirm, ok := m.topModal().(*confirmModal)
	if cmd != nil || !ok {
		t.Fatal("d should ask before closing the read")
	}
	if !strings.Contains(confirm.c.Message, model.StatusDidNotFinish.Label()) {
		t.Fatalf("confirm message = %q, want it to name the new status", confirm.c.Message)
	}
}

func TestConfirmationDoesNotSwallowAsyncResults(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setSection(sectionReading)

	send(t, m, runeKey('x'))
	m.inflight = 1

	m.Update(libraryLoadedMsg{
		reading: []model.UserBook{{Book: model.Book{ID: 9, Title: "Ubik"}}},
	})

	if _, ok := m.topModal().(*confirmModal); !ok {
		t.Fatal("an async result must not close the confirmation")
	}
	if len(m.shared.reading) != 1 || m.shared.reading[0].Book.Title != "Ubik" {
		t.Fatalf("reading = %#v, want the load applied behind the confirmation", m.shared.reading)
	}
	if m.isLoading() {
		t.Fatal("the finished load should have released its slot")
	}
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
	setLibrary(m, m.shared.reading, books)
	m.setSection(sectionOku)

	badge := okuSection(m).overflowBadge()
	if badge != "1/30" {
		t.Fatalf("overflow badge = %q, want 1/30", badge)
	}
	if !strings.Contains(stripANSI(m.frame()), badge) {
		t.Fatal("the Oku card should show the overflow badge")
	}

	// A list that fits says nothing.
	setLibrary(m, m.shared.reading, books[:1])
	if got := okuSection(m).overflowBadge(); got != "" {
		t.Fatalf("overflow badge = %q, want none when the list fits", got)
	}

	// The focused card's rows are padded out to its full width, so the badge
	// has to overwrite the tail rather than be appended to it.
	small := renderedDashboard(80, 24)
	setLibrary(small, books[:5], small.shared.oku)
	small.setSection(sectionReading)
	if got := readingSection(small).overflowBadge(); got != "1/5" {
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

		w := m.lay.rightPanelContentWidth()
		detail := detailsView(m.section().Selected().Book, m.shared.density, w, m.st)
		for _, line := range strings.Split(stripANSI(detail), "\n") {
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
	m.shared.timer = &model.TimerState{BookID: 1, StartedAt: time.Now()}

	if cmd := send(t, m, runeKey('t')); cmd == nil {
		t.Fatal("t should stop the running timer in the Timer section too")
	}
	if m.timerPicker() != nil {
		t.Fatal("stopping must not open the book picker")
	}
}

func TestSecondTimerPressIsGuardedWhileInFlight(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setSection(sectionReading)

	if cmd := send(t, m, runeKey('t')); cmd == nil {
		t.Fatal("t should start a timer")
	}
	before := m.inflight

	send(t, m, runeKey('t'))
	if m.inflight != before {
		t.Fatal("a second t before the first result returns must not start another session")
	}
	if !strings.Contains(m.toast.text, "in flight") {
		t.Fatalf("toast = %q, want the in-flight notice", m.toast.text)
	}
}

func TestSearchHeaderNamesTheResultsOnScreen(t *testing.T) {
	m := newTestModel()
	m.setSection(sectionSearch)
	m.search.input.SetValue("dune")
	m.inflight = 1

	send(t, m, searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}, {ID: 2, Title: "Dune Messiah"}},
		query:   "dune",
		mode:    model.SearchModeBook,
		seq:     m.search.seq,
	})
	if m.search.list.Title != "BOOK Results (2)" {
		t.Fatalf("title = %q, want BOOK Results (2)", m.search.list.Title)
	}

	// Switching the mode does not rename results fetched in the old one.
	send(t, m, m.search.setQueryMode(model.SearchModeAuthor))
	if m.search.list.Title != "BOOK Results (2)" {
		t.Fatalf("title after switching mode = %q, want the results' own mode", m.search.list.Title)
	}

	// A failed search leaves the previous results, and their count, named.
	m.inflight = 1
	m.search.loading = true
	send(t, m, searchLoadedMsg{
		query: "dune",
		mode:  model.SearchModeAuthor,
		seq:   m.search.seq,
		err:   errors.New("network down"),
	})
	if m.search.list.Title != "BOOK Results (2)" {
		t.Fatalf("title after a failed search = %q, want the results still on screen", m.search.list.Title)
	}
}

func TestThemeResolvesDistinctColoursForLightAndDark(t *testing.T) {
	withColorProfile(t)

	th := DefaultTheme()
	colours := map[string]lipgloss.AdaptiveColor{
		"accent": th.Accent, "heading": th.Heading, "text": th.Text,
		"textMuted": th.TextMuted, "textDim": th.TextDim, "border": th.Border,
		"borderFocused": th.BorderFocused, "surface": th.Surface,
		"success": th.Success, "warning": th.Warning, "error": th.Error,
		"heat1": th.Heat1, "heat2": th.Heat2, "heat3": th.Heat3, "heat4": th.Heat4,
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
		for _, c := range []lipgloss.AdaptiveColor{th.Heat1, th.Heat2, th.Heat3, th.Heat4} {
			if seen[side.pick(c)] {
				t.Fatalf("%s heat ramp repeats %q", side.name, side.pick(c))
			}
			seen[side.pick(c)] = true
		}
	}
}

func TestApplyThemeSetting(t *testing.T) {
	withColorProfile(t)

	if err := ApplyThemeSetting("light"); err != nil {
		t.Fatalf("ApplyThemeSetting(light) error = %v", err)
	}
	if lipgloss.HasDarkBackground() {
		t.Fatal("theme = light should pin a light background")
	}
	if err := ApplyThemeSetting("Dark"); err != nil {
		t.Fatalf("ApplyThemeSetting(Dark) error = %v", err)
	}
	if !lipgloss.HasDarkBackground() {
		t.Fatal("theme = dark should pin a dark background")
	}
	for _, ok := range []string{"", "auto", " AUTO "} {
		if err := ApplyThemeSetting(ok); err != nil {
			t.Fatalf("ApplyThemeSetting(%q) error = %v, want none", ok, err)
		}
	}
	if err := ApplyThemeSetting("solarized"); err == nil {
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
// keys: every one has to change the screen or start something. A binding
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
	base := func(w, h int) *Model {
		m := renderedDashboard(w, h)
		setLibrary(m, books(1, 2, 3), books(4, 5, 6))
		readingSection(m).list.Select(1)
		okuSection(m).list.Select(1)
		m.shared.stats, _ = demoLocalData(time.Now())
		return m
	}
	withResults := func(m *Model) *Model {
		m.search.results = []model.SearchResult{{ID: 7, Title: "Dune"}, {ID: 8, Title: "Dune Messiah"}, {ID: 9, Title: "Children of Dune"}}
		m.search.rebuildResults()
		m.search.list.Select(1)
		m.search.input.SetValue("dune")
		return m
	}
	inSection := func(s focusSection) func() *Model {
		return func() *Model {
			m := base(120, 40)
			m.setSection(s)
			return m
		}
	}
	confirmWithCursor := func(cursor int) func() *Model {
		return func() *Model {
			m := inSection(sectionReading)()
			send(t, m, runeKey('x'))
			m.topModal().(*confirmModal).c.Cursor = cursor
			return m
		}
	}

	states := []struct {
		name string
		// variants are arrangements of the same focus; a key passes when it
		// does something in any of them (a two-button dialog cannot move
		// left and right from one cursor).
		variants []func() *Model
	}{
		{"intro", []func() *Model{inSection(sectionIntro)}},
		{"reading", []func() *Model{inSection(sectionReading)}},
		{"oku", []func() *Model{inSection(sectionOku)}},
		{"search input, normal mode", []func() *Model{func() *Model {
			m := withResults(inSection(sectionSearch)())
			m.search.sub = searchSubInput
			m.search.enterNormalMode()
			return m
		}}},
		{"search input, insert mode", []func() *Model{func() *Model {
			m := withResults(inSection(sectionSearch)())
			m.search.sub = searchSubInput
			m.search.enterInsertMode()
			return m
		}}},
		{"search results", []func() *Model{func() *Model {
			m := withResults(inSection(sectionSearch)())
			m.search.sub = searchSubResults
			m.search.enterNormalMode()
			return m
		}}},
		{"stats", []func() *Model{func() *Model {
			// One row down: the page can still scroll either way.
			m := inSection(sectionStats)()
			m.sections[sectionStats].(*statsSection).scroll = 1
			return m
		}}},
		{"timer, idle", []func() *Model{inSection(sectionTimer)}},
		{"timer, picking a book", []func() *Model{func() *Model {
			m := inSection(sectionTimer)()
			m.push(newTimerPickerModal(m.shared, 1))
			return m
		}}},
		{"timer, running", []func() *Model{func() *Model {
			m := inSection(sectionTimer)()
			m.shared.timer = &model.TimerState{BookID: 1, StartedAt: time.Now()}
			return m
		}}},
		{"help modal", []func() *Model{func() *Model {
			// Short enough that the body scrolls, scrolled one row so every
			// direction has somewhere to go.
			m := base(80, 24)
			m.setSection(sectionReading)
			m.openHelp()
			m.topModal().(*helpModal).vp.SetYOffset(1)
			return m
		}}},
		{"confirm", []func() *Model{confirmWithCursor(1), confirmWithCursor(0)}},
		{"page prompt", []func() *Model{func() *Model {
			m := inSection(sectionReading)()
			m.push(newPageModal(m.shared, m.st, m.shared.reading[1]))
			return m
		}}},
		{"review modal", []func() *Model{func() *Model {
			m := inSection(sectionReading)()
			openReview(m, m.shared.reading[1]).rating.SetValue("3")
			return m
		}}},
		{"undo on offer", []func() *Model{func() *Model {
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
						cmd := send(t, m, msg)
						if cmd != nil || stripANSI(m.frame()) != before {
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

// helpBody is the help modal's rows for the focus m has now.
func helpBody(m *Model) string {
	return newHelpModal(m.keysBehind, m.st).body()
}

// TestHelpModalListsEveryGroupWithTheActiveOnesFirst checks the modal still
// teaches the other sections' keys: from Reading it names the search keys,
// dimmed, after the groups that apply.
func TestHelpModalListsEveryGroupWithTheActiveOnesFirst(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setSection(sectionReading)
	body := stripANSI(helpBody(m))

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
	raw := helpBody(m)
	live := m.st.modalKey.Width(12).Render("u")
	if !strings.Contains(raw, live) {
		t.Fatal("a live key should be drawn in the key style")
	}
	if strings.Contains(raw, m.st.modalKey.Width(12).Render("i")) {
		t.Fatal("a key the focus does not understand should not be drawn as live")
	}
}

// TestOverloadedKeysMatchTheirHelp pins the help-modal labels of keys that
// mean different things in different places to what the handler does there.
func TestOverloadedKeysMatchTheirHelp(t *testing.T) {
	overResults := func() *Model {
		m := renderedDashboard(120, 40)
		m.setSection(sectionSearch)
		m.search.results = []model.SearchResult{{ID: 1, Title: "Dune"}}
		m.search.rebuildResults()
		m.search.sub = searchSubResults
		m.search.enterNormalMode()
		return m
	}

	// Over the search results h goes back to the input, so the section hint
	// must not claim it.
	m := overResults()
	k := m.activeKeys()
	rows := stripANSI(helpBody(m))
	send(t, m, runeKey('h'))
	if m.search.sub != searchSubInput || m.tab != sectionSearch {
		t.Fatalf("h over the results: searchSub=%v section=%v, want back to the input", m.search.sub, m.tab)
	}
	for _, key := range k.PrevSection.Keys() {
		if key == "h" || key == "left" {
			t.Fatalf("PrevSection still claims %q over the results", key)
		}
	}
	// The Confirm group (dimmed) still lists "h/l pick a button"; it is the
	// section row that must not claim h here.
	if !strings.Contains(rows, "Esc/h") || !strings.Contains(rows, "S-Tab/l") || strings.Contains(rows, "h/l           section") {
		t.Fatalf("results help should say Esc/h goes back and S-Tab goes to the previous section:\n%s", rows)
	}
	m = overResults()
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.tab != sectionOku {
		t.Fatalf("shift+tab over the results: section=%v, want the previous section", m.tab)
	}

	// In Intro k goes to the previous section, so j/k cannot be labelled
	// "next section".
	intro := func() *Model {
		m := renderedDashboard(120, 40)
		m.setSection(sectionIntro)
		return m
	}
	up := intro()
	send(t, up, runeKey('k'))
	if up.tab != sectionTimer {
		t.Fatalf("k in Intro: section=%v, want the previous section (Timer)", up.tab)
	}
	down := intro()
	send(t, down, runeKey('j'))
	if down.tab != sectionReading {
		t.Fatalf("j in Intro: section=%v, want the next section (Reading)", down.tab)
	}
	fresh := intro()
	if rows := stripANSI(helpBody(fresh)); strings.Contains(rows, "j/k           next section") || !strings.Contains(rows, "j/k           section") {
		t.Fatalf("Intro help should label j/k as moving between sections, not one way:\n%s", rows)
	}
	if d := fresh.activeKeys().upDownDesc(); d != "section" {
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
	m.Update(opDoneMsg{op: opSync, err: errors.New("offline")})
	if m.undo != nil {
		t.Fatal("a later toast must drop the pending undo")
	}
	if m.activeKeys().Undo.Enabled() {
		t.Fatal("U should not be advertised once its toast is gone")
	}
	before := m.inflight
	if cmd := send(t, m, runeKey('U')); cmd != nil || m.inflight != before {
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
	m.Update(toastExpiredMsg{seq: firstSeq})
	if m.toast.text != "second" {
		t.Fatalf("toast = %+v, want the newer toast left alone by the old tick", m.toast)
	}

	m.Update(toastExpiredMsg{seq: m.toast.seq})
	if m.toast.text != "" {
		t.Fatalf("toast = %+v, want it cleared by its own tick", m.toast)
	}
	if strings.Contains(stripANSI(m.frame()), "second") {
		t.Fatal("an expired toast should leave the status bar")
	}

	// Errors get longer than notes, and the tick is a real Bubble Tea tick.
	if toastErrorTTL <= toastTTL {
		t.Fatalf("error TTL %v should outlast the info TTL %v", toastErrorTTL, toastTTL)
	}
}

func TestStatusChangeOffersUndoWhileTheToastIsUp(t *testing.T) {
	moved := func() *Model {
		m := renderedDashboard(120, 40)
		m.setSection(sectionReading)
		m.inflight = 1
		m.Update(opDoneMsg{
			op: opStatus, info: "Status changed to Read", reload: true, markDirty: true,
			bookID: 1, title: "Dune", prevStatus: model.StatusCurrentlyReading, newStatus: model.StatusRead,
		})
		return m
	}

	m := moved()
	if m.undo == nil || m.undo.op != opStatus || m.undo.bookID != 1 ||
		m.undo.toStatus != model.StatusCurrentlyReading || m.undo.fromStatus != model.StatusRead {
		t.Fatalf("undo = %+v, want the way back to Currently Reading for Dune", m.undo)
	}
	bar := stripANSI(m.statusBar())
	if !strings.Contains(bar, "Moved 'Dune' to Read") || !strings.Contains(bar, "U undo") {
		t.Fatalf("status bar = %q, want the move and the undo hint", bar)
	}
	if !m.activeKeys().Undo.Enabled() {
		t.Fatal("U should be live while the undo is on offer")
	}

	// The reload the result started is still in flight; undo must not wait
	// for it, the status it sets is absolute.
	before := m.inflight
	if cmd := send(t, m, runeKey('U')); cmd == nil || m.inflight != before+1 {
		t.Fatalf("U should start the reverse status change (inflight %d → %d)", before, m.inflight)
	}
	if m.undo != nil {
		t.Fatal("an undo can only be taken once")
	}

	// Once the toast has expired, U does nothing.
	expired := moved()
	expired.Update(toastExpiredMsg{seq: expired.toast.seq})
	if expired.activeKeys().Undo.Enabled() {
		t.Fatal("U should not be advertised once the undo is gone")
	}
	before = expired.inflight
	if cmd := send(t, expired, runeKey('U')); cmd != nil || expired.inflight != before {
		t.Fatal("U after the toast expired must not change anything")
	}
}

func TestQuickProgressOffersUndoToThePreviousPage(t *testing.T) {
	updated := func(msg opDoneMsg) *Model {
		m := renderedDashboard(120, 40)
		m.setSection(sectionReading)
		m.inflight = 1
		m.Update(msg)
		return m
	}

	m := updated(opDoneMsg{
		op: opProgress, info: "Progress +10 → page 130", reload: true, markDirty: true,
		bookID: 1, title: "Dune", prevPage: 120, newPage: 130,
	})

	if m.undo == nil || m.undo.op != opProgress || m.undo.toPage != 120 || m.undo.fromPage != 130 {
		t.Fatalf("undo = %+v, want the way back to page 120", m.undo)
	}
	if bar := stripANSI(m.statusBar()); !strings.Contains(bar, "Page 130") || !strings.Contains(bar, "U undo") {
		t.Fatalf("status bar = %q, want the page and the undo hint", bar)
	}

	before := m.inflight
	if cmd := send(t, m, runeKey('U')); cmd == nil || m.inflight != before+1 {
		t.Fatal("U should start the update back to the previous page")
	}

	// A failed operation and one that changed nothing offer no undo.
	failed := updated(opDoneMsg{op: opProgress, err: errors.New("offline"), bookID: 1, prevPage: 120, newPage: 130})
	if failed.undo != nil || failed.toast.level != toastError {
		t.Fatalf("a failed update offered undo or hid the error: %+v", failed.toast)
	}
	same := updated(opDoneMsg{op: opProgress, info: "Progress updated to page 120", bookID: 1, prevPage: 120, newPage: 120})
	if same.undo != nil {
		t.Fatal("an update that changed nothing has nothing to undo")
	}
}

func TestUndoNeverStealsALetterFromTheSearchInput(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.showUndoToast("Moved 'Dune' to Read", undoAction{op: opStatus, bookID: 1,
		fromStatus: model.StatusRead, toStatus: model.StatusCurrentlyReading})
	m.setSection(sectionSearch)
	m.search.sub = searchSubInput
	m.search.enterInsertMode()

	m.Update(runeKey('U'))
	if m.search.input.Value() != "U" {
		t.Fatalf("search input = %q, want the letter typed", m.search.input.Value())
	}
	if m.undo == nil {
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
	m.search.sub = searchSubInput
	inputFrame := ansi.Strip(m.frame())
	m.search.sub = searchSubResults
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

func TestSharedClockDrivesEveryRender(t *testing.T) {
	fixed := time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local)
	m := renderedDashboard(120, 40)
	m.shared.now = func() time.Time { return fixed }
	m.shared.timer = &model.TimerState{BookID: 1, StartedAt: fixed.Add(-90 * time.Second)}
	m.shared.timerBook = &m.shared.reading[0].Book
	m.shared.stats, m.shared.sessions = demoLocalData(fixed)

	m.setSection(sectionTimer)
	frame := stripANSI(m.frame())
	if !strings.Contains(frame, "00:01:30") {
		t.Fatalf("the timer should be read off the shared clock:\n%s", frame)
	}
	if !strings.Contains(frame, "Today") {
		t.Fatalf("sessions should be dated against the shared clock:\n%s", frame)
	}
	m.setSection(sectionIntro)
	if got := stripANSI(m.frame()); !strings.Contains(got, "1m 30s") {
		t.Fatalf("the intro's timer should be read off the shared clock:\n%s", got)
	}
}
