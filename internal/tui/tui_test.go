package tui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/x/ansi"
)

// setLibrary puts books on the shelves the way a load would and rebuilds the
// lists from them.
func setLibrary(m *Model, reading, oku []model.UserBook) {
	m.shared.reading, m.shared.oku = reading, oku
	m.broadcast(dataChangedMsg{dataLibrary})
}

// openReview puts the review modal up for book and returns it.
func openReview(m *Model, book model.UserBook) *reviewModal {
	m.push(newReviewModal(m.shared, m.st, book))
	return m.topModal().(*reviewModal)
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

func TestLibrarySectionVimNavigation(t *testing.T) {
	m := newTestModel()
	m.setTab(tabReading)

	m.Update(runeKey('l'))
	if m.tab != tabOku {
		t.Fatalf("section after l = %v, want %v", m.tab, tabOku)
	}

	m.Update(runeKey('h'))
	if m.tab != tabReading {
		t.Fatalf("section after h = %v, want %v", m.tab, tabReading)
	}
}

func TestSubmitSearchSetsLoadingState(t *testing.T) {
	m := newTestModel()
	m.setTab(tabSearch)
	searchOf(m).focusInput()
	searchOf(m).input.SetValue("dune")
	searchOf(m).queryMode = model.SearchModeAuthor

	cmd := send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submitting a non-empty query should start a search")
	}
	if !m.isLoading() || !searchOf(m).loading {
		t.Fatalf("loading flags = loading:%v searchLoading:%v, want true/true", m.isLoading(), searchOf(m).loading)
	}

	if searchOf(m).loadingQuery != "dune" {
		t.Fatalf("loadingQuery = %q, want dune", searchOf(m).loadingQuery)
	}
	if got := stripANSI(searchOf(m).controlRow(60)); !strings.Contains(got, "searching") {
		t.Fatalf("control row %q does not say a search is running", got)
	}
	if !strings.Contains(strings.ToLower(m.toast.text), "searching for") {
		t.Fatalf("toast %q does not include searching feedback", m.toast.text)
	}
}

func TestSubmitSearchGuardAndEmptyValidation(t *testing.T) {
	m := newTestModel()
	m.setTab(tabSearch)
	searchOf(m).focusInput()
	searchOf(m).loading = true
	searchOf(m).input.SetValue("dune")
	if cmd := send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
		t.Fatal("a query typed over an in-flight search should be searchable")
	}

	searchOf(m).input.SetValue("   ")
	m.inflight = 0
	searchOf(m).loading = false

	send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.isLoading() || searchOf(m).loading {
		t.Fatal("an empty query should not be searched for")
	}
	if m.toast.level != toastError || m.toast.text == "" {
		t.Fatalf("toast = %+v, want a validation error for the empty query", m.toast)
	}
}

func TestSearchLoadedMsgTransitionsToResults(t *testing.T) {
	m := newTestModel()
	m.inflight = 1
	m.setTab(tabSearch)
	searchOf(m).loading = true
	searchOf(m).focusInput()
	searchOf(m).queryMode = model.SearchModeAuthor

	send(t, m, searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}},
		query:   "dune",
		mode:    model.SearchModeAuthor,
	})

	if m.isLoading() || searchOf(m).loading {
		t.Fatalf("loading flags after response = loading:%v searchLoading:%v, want false/false", m.isLoading(), searchOf(m).loading)
	}

	if searchOf(m).focus != resultsFocused {
		t.Fatalf("search focus = %v, want the results", searchOf(m).focus)
	}
	if searchOf(m).lastQuery != "dune" {
		t.Fatalf("lastQuery = %q, want dune", searchOf(m).lastQuery)
	}
	if searchOf(m).lastMode != model.SearchModeAuthor {
		t.Fatalf("lastMode = %q, want %q", searchOf(m).lastMode, model.SearchModeAuthor)
	}

	if got := stripANSI(searchOf(m).controlRow(60)); !strings.Contains(got, "1 results") {
		t.Fatalf("control row = %q, want it to count the results", got)
	}
	if !strings.Contains(m.toast.text, "loaded 1 results") {
		t.Fatalf("toast = %q, expected loaded-count feedback", m.toast.text)
	}
}

func TestSlashSearchPreservesExistingQuery(t *testing.T) {
	m := newTestModel()
	m.setTab(tabReading)
	searchOf(m).input.SetValue("dune")

	m.Update(runeKey('/'))

	if m.tab != tabSearch {
		t.Fatalf("section after / = %v, want %v", m.tab, tabSearch)
	}
	if searchOf(m).focus != inputFocused {
		t.Fatalf("search focus after / = %v, want the input", searchOf(m).focus)
	}
	if searchOf(m).input.Value() != "dune" {
		t.Fatalf("search query after / = %q, want %q", searchOf(m).input.Value(), "dune")
	}
}

func TestTimerStartOpensBookSelectionFirst(t *testing.T) {
	m := newTestModel()
	m.setTab(tabTimer)
	setLibrary(m, []model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune"}},
		{Book: model.Book{ID: 2, Title: "Foundation"}},
	}, nil)

	send(t, m, runeKey('t'))

	if m.isLoading() {
		t.Fatal("timer start should open selection first, got an immediate operation")
	}
	if timerPickerOf(m) == nil {
		t.Fatal("the picker should be up after pressing t")
	}
}

func TestCycleDensityRefreshesSearchResultItems(t *testing.T) {
	m := newTestModel()
	m.shared.density = DensityDefault
	searchOf(m).results = []model.SearchResult{
		{ID: 1, Title: "Dune", Authors: []string{"Frank Herbert"}, Slug: "dune"},
	}
	searchOf(m).rebuildResults()

	send(t, m, runeKey('z'))

	items := searchOf(m).list.Items()
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

	cmd := send(t, m, tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})

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
	m.setTab(tabTimer)
	setLibrary(m, []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}}, nil)
	// Stale: the list shrank while the picker was open.
	m.push(newTimerPickerModal(m.shared, 5))

	cmd := send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("enter on a valid list should start the timer")
	}
	if timerPickerOf(m) != nil {
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
			pageModalOf(m).input.SetValue("120")
		}},
		{name: "review modal", open: func(m *Model) { openReview(m, book) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel()
			m.shared.loaded = true
			m.setTab(tabSearch)
			searchOf(m).focusInput()
			searchOf(m).input.SetValue("dune")
			if cmd := send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
				t.Fatal("submitting the query should start a search")
			}

			tt.open(m)
			top := m.topModal()

			send(t, m, searchLoadedMsg{
				results: []model.SearchResult{{ID: 1, Title: "Dune"}},
				query:   "dune",
				mode:    model.SearchModeBook,
				seq:     searchOf(m).seq,
			})

			if searchOf(m).loading {
				t.Fatal("searchLoading should be cleared by the result, whatever the mode")
			}
			if m.isLoading() {
				t.Fatal("loading should be cleared by the result, whatever the mode")
			}
			if len(searchOf(m).results) != 1 {
				t.Fatalf("results = %d, want 1", len(searchOf(m).results))
			}
			if m.topModal() != top {
				t.Fatalf("top modal = %T, want %T: an async result must not close a modal", m.topModal(), top)
			}
			if searchOf(m).focus != resultsFocused {
				t.Fatalf("search focus = %v, want the results, so j/k move through them", searchOf(m).focus)
			}
			if page := pageModalOf(m); page != nil && page.input.Value() != "120" {
				t.Fatalf("page input = %q, want it untouched", page.input.Value())
			}
		})
	}
}

func TestStaleSearchResultIsIgnored(t *testing.T) {
	m := newTestModel()
	m.setTab(tabSearch)
	searchOf(m).focusInput()
	searchOf(m).input.SetValue("dune")
	if cmd := send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
		t.Fatal("the first query should start a search")
	}
	staleSeq := searchOf(m).seq

	// The user retypes before the first response lands.
	searchOf(m).loading = false
	searchOf(m).input.SetValue("foundation")
	if cmd := send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
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
	if !searchOf(m).loading {
		t.Fatal("a superseded result must not clear the in-flight search")
	}
	if len(searchOf(m).results) != 0 {
		t.Fatalf("results = %d, want 0 for a superseded result", len(searchOf(m).results))
	}

	send(t, m, searchLoadedMsg{
		results: []model.SearchResult{{ID: 2, Title: "Foundation"}},
		query:   "foundation",
		mode:    model.SearchModeBook,
		seq:     searchOf(m).seq,
	})

	if searchOf(m).loading {
		t.Fatal("the latest result should clear searchLoading")
	}
	if searchOf(m).lastQuery != "foundation" {
		t.Fatalf("lastQuery = %q, want foundation", searchOf(m).lastQuery)
	}
}

func TestSearchResultKeepsUserSelectedMode(t *testing.T) {
	m := newTestModel()
	m.setTab(tabSearch)
	searchOf(m).queryMode = model.SearchModeGenre

	send(t, m, searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}},
		query:   "dune",
		mode:    model.SearchModeBook,
		seq:     searchOf(m).seq,
	})

	if searchOf(m).queryMode != model.SearchModeGenre {
		t.Fatalf("queryMode = %q, want the mode the user picked (%q)", searchOf(m).queryMode, model.SearchModeGenre)
	}
	if got := stripANSI(searchOf(m).controlRow(60)); !strings.Contains(got, "[Genre]") {
		t.Fatalf("control row = %q, want the mode the user picked marked as the active one", got)
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
	if timerPickerOf(m) != nil {
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
	m.setTab(tabReading)
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

	send(t, m, tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})
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

	send(t, m, tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})
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
	m.setTab(tabReading)
	setLibrary(m, []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}}, nil)

	send(t, m, runeKey('+')) // progress update in flight
	send(t, m, runeKey('u')) // user opens the page modal
	if pageModalOf(m) == nil {
		t.Fatalf("top modal = %T, want the page prompt open", m.topModal())
	}

	m.Update(opDoneMsg{op: opProgress, info: "Progress +10 → page 40", markDirty: true})

	if pageModalOf(m) == nil {
		t.Fatal("a progress result started before the modal opened must not close it")
	}
}

func TestPageModalEnterIsGuardedWhileInFlight(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.setTab(tabReading)
	setLibrary(m, []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}}, nil)

	send(t, m, runeKey('+'))
	send(t, m, runeKey('u'))
	pageModalOf(m).input.SetValue("120")

	send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.inflight != 1 {
		t.Fatalf("inflight = %d: enter must not submit a page update while one is in flight", m.inflight)
	}
	page := pageModalOf(m)
	if page == nil {
		t.Fatal("the modal should stay open after a refused submission")
	}
	if m.toast.text == "" {
		t.Fatal("the refused submission should say why")
	}
	// The prompt is drawn over a blank screen, so that toast cannot be read:
	// the refusal has to reach the panel, and it must not leave it saving.
	if page.submitting {
		t.Fatal("a refused submission should not leave the prompt saving for ever")
	}
	if !strings.Contains(page.err, inFlightNotice) {
		t.Fatalf("prompt error = %q, want the reason it was refused", page.err)
	}
	if page.input.Value() != "120" {
		t.Fatalf("input = %q, want the typed page kept", page.input.Value())
	}
}

// TestModalIsComposedOverTheDashboard: the panel is drawn onto the frame
// rather than over a blanked screen, so the header and both panes are still
// on the terminal behind it.
func TestModalIsComposedOverTheDashboard(t *testing.T) {
	m := renderedDashboard(120, 40)
	setLibrary(m, []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune", Pages: 300}}}, nil)
	behind := m.View().Content

	m.push(newPageModal(m.shared, m.st, m.shared.reading[0]))
	over := m.View().Content
	if over == behind {
		t.Fatal("the modal changed nothing on screen")
	}
	for _, want := range []string{"oku", "Reading (1)", "Update page"} {
		if !strings.Contains(stripANSI(over), want) {
			t.Fatalf("the modal frame is missing %q:\n%s", want, stripANSI(over))
		}
	}
}

// TestPageModalPutsTheRealCursorInItsInput: the prompt draws no block of its
// own, so the terminal's cursor has to land inside the placed panel.
func TestPageModalPutsTheRealCursorInItsInput(t *testing.T) {
	m := renderedDashboard(120, 40)
	setLibrary(m, []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune", Pages: 300}}}, nil)
	m.push(newPageModal(m.shared, m.st, m.shared.reading[0]))

	cur := m.View().Cursor
	if cur == nil {
		t.Fatal("the page prompt has no cursor")
	}
	// The panel is centred, so the cursor is inside it rather than at the
	// input's own origin.
	panel := m.topModal().View(m.lay, m.st)
	x := (m.lay.W - lipgloss.Width(panel)) / 2
	y := (m.lay.H - lipgloss.Height(panel)) / 2
	if cur.X <= x || cur.Y <= y {
		t.Fatalf("cursor = (%d,%d), want it inside the panel placed at (%d,%d)", cur.X, cur.Y, x, y)
	}
	if cur.Y != y+modalContentY+2 {
		t.Fatalf("cursor row = %d, want the input's row %d", cur.Y, y+modalContentY+2)
	}
}

// TestPageModalIsReadOnlyWhileSaving is the review modal's rule for the page
// prompt: the fields stop taking keys until the save reports back, so a
// second Enter cannot start a second update, and Esc still cancels.
func TestPageModalIsReadOnlyWhileSaving(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.push(newPageModal(m.shared, m.st, model.UserBook{Book: model.Book{ID: 1, Title: "Dune"}}))
	page := pageModalOf(m)
	page.input.SetValue("120")

	send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !page.submitting || m.inflight != 1 {
		t.Fatalf("submitting=%v inflight=%d, want the save started", page.submitting, m.inflight)
	}

	send(t, m, runeKey('9'))
	if page.input.Value() != "120" {
		t.Fatalf("input = %q, want it read-only while saving", page.input.Value())
	}
	send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.inflight != 1 {
		t.Fatalf("inflight = %d, want the second Enter to have started nothing", m.inflight)
	}

	send(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.topModal() != nil {
		t.Fatalf("top modal = %T, want Esc to cancel out of a saving prompt", m.topModal())
	}
}

// TestReviewSaveIsGuardedWhileInFlight gives the review modal the page
// prompt's guard: a save started over another operation is refused, and the
// reason lands in the panel rather than in a status bar behind it.
func TestReviewSaveIsGuardedWhileInFlight(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.setTab(tabReading)
	setLibrary(m, []model.UserBook{{Book: model.Book{ID: 1, Title: "Dune"}}}, nil)

	send(t, m, runeKey('+')) // a progress update in flight
	review := openReview(m, m.shared.reading[0])
	review.rating.SetValue("4")

	send(t, m, tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})

	if m.inflight != 1 {
		t.Fatalf("inflight = %d: the save must not start over another operation", m.inflight)
	}
	if m.topModal() != review {
		t.Fatalf("top modal = %T, want the review modal still open", m.topModal())
	}
	if review.submitting {
		t.Fatal("a refused save should not leave the modal saving for ever")
	}
	if !strings.Contains(review.err, inFlightNotice) {
		t.Fatalf("modal error = %q, want the reason it was refused", review.err)
	}
}

func TestPageModalClosesOnItsOwnResult(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.push(newPageModal(m.shared, m.st, model.UserBook{Book: model.Book{ID: 1, Title: "Dune"}}))
	page := pageModalOf(m)
	page.input.SetValue("120")

	cmd := send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.isLoading() {
		t.Fatal("enter should submit the page update")
	}

	m.Update(opDoneMsg{op: opProgress, seq: page.token, info: "Progress updated to page 120", markDirty: true})

	if pageModalOf(m) != nil {
		t.Fatal("the modal's own result should close it")
	}
}

// TestPageModalFailureKeepsModalOpen pins the page prompt to the review
// modal's rule: a save that comes back with an error leaves the prompt up,
// with what was typed still in it and the reason on screen. The status bar
// sits behind the overlay, so closing on the failure hid it entirely.
func TestPageModalFailureKeepsModalOpen(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.push(newPageModal(m.shared, m.st, model.UserBook{Book: model.Book{ID: 1, Title: "Dune"}}))
	page := pageModalOf(m)
	page.input.SetValue("120")

	send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !page.submitting {
		t.Fatal("enter should mark the prompt as saving")
	}

	m.Update(opDoneMsg{op: opProgress, seq: page.token, err: errors.New("network down")})

	if pageModalOf(m) == nil {
		t.Fatal("a failed save must not close the prompt")
	}
	if page.submitting {
		t.Fatal("a failed save should clear the saving state")
	}
	if page.input.Value() != "120" {
		t.Fatalf("input = %q, want the typed page kept", page.input.Value())
	}
	if !strings.Contains(stripANSI(page.View(m.lay, m.st)), "network down") {
		t.Fatalf("the prompt should show the failure:\n%s", stripANSI(page.View(m.lay, m.st)))
	}

	// The corrected save closes it.
	send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(opDoneMsg{op: opProgress, seq: page.token, info: "Progress updated to page 120"})
	if m.topModal() != nil {
		t.Fatalf("top modal = %T, want the prompt closed by a successful save", m.topModal())
	}
}

func TestModalStackPopsOnOwnResultOnly(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	m.push(newPageModal(m.shared, m.st, model.UserBook{Book: model.Book{ID: 1, Title: "Dune"}}))
	page := pageModalOf(m)

	// A progress result from elsewhere, and another operation stamped with
	// the same token, are not the prompt's own.
	m.Update(opDoneMsg{op: opProgress, seq: page.token + 1, info: "Progress +10 → page 40"})
	if pageModalOf(m) == nil {
		t.Fatal("another session's progress result must not close the prompt")
	}
	m.Update(opDoneMsg{op: opReview, seq: page.token, info: "Updated rating"})
	if pageModalOf(m) == nil {
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

func TestReviewModalIsReadOnlyWhileSaving(t *testing.T) {
	m := newTestModel()
	m.shared.loaded = true
	review := openReview(m, model.UserBook{Book: model.Book{ID: 42, Title: "Dune"}})
	review.rating.SetValue("3")
	review.text.SetValue("Strong first half.")

	send(t, m, tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'})
	pendingSeq := review.token

	m.Update(runeKey('x'))
	if review.text.Value() != "Strong first half." {
		t.Fatalf("review text = %q, want it read-only while saving", review.text.Value())
	}

	// Cancelling drops the pending result instead of reopening the modal.
	send(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
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
	m.setTab(tabSearch)
	searchOf(m).focusInput()
	searchOf(m).input.SetValue("dune")
	if cmd := send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
		t.Fatal("submitting the query should start a search")
	}

	send(t, m, searchLoadedMsg{
		query: "dune",
		mode:  model.SearchModeBook,
		seq:   searchOf(m).seq,
		err:   errors.New("network down"),
	})

	if got := stripANSI(searchOf(m).controlRow(60)); strings.Contains(got, "searching") {
		t.Fatalf("control row = %q, want the search over", got)
	}
	if searchOf(m).loading {
		t.Fatal("a failed search should clear searchLoading")
	}
}

// fixedNow is the clock every rendered dashboard reads, so a frame is the
// same bytes whenever the test runs.
var fixedNow = time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local)

// renderedDashboard builds a loaded dashboard of the given size with a couple
// of books on each shelf, ready to render.
func renderedDashboard(w, h int) *Model {
	m := newTestModel()
	m.shared.loaded = true
	m.shared.now = func() time.Time { return fixedNow }
	setLibrary(m, []model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune", Pages: 412}, CurrentPage: 120},
		{Book: model.Book{ID: 2, Title: "Foundation", Pages: 255}},
	}, []model.UserBook{
		{Book: model.Book{ID: 3, Title: "Meditations"}},
	})
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

// allTabs is every tab, for the tests that walk them.
var allTabs = []tab{tabReading, tabOku, tabSearch, tabStats, tabTimer}

func TestViewFillsTerminalExactly(t *testing.T) {
	book := model.UserBook{Book: model.Book{ID: 1, Title: "Dune", Pages: 412}, CurrentPage: 120}
	overlays := map[string]func(m *Model){
		"none":         func(*Model) {},
		"help":         func(m *Model) { m.openHelp() },
		"page":         func(m *Model) { m.push(newPageModal(m.shared, m.st, book)) },
		"review":       func(m *Model) { m.push(newReviewModal(m.shared, m.st, book)) },
		"confirm":      func(m *Model) { m.push(newConfirmModal("Mark 'Dune' as Ignored?", nil)) },
		"timer picker": func(m *Model) { m.push(newTimerPickerModal(m.shared, 0)) },
	}

	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 40}, {60, 20}} {
		w, h := size[0], size[1]
		for _, tb := range allTabs {
			for _, focus := range []focus{focusContent, focusDetail} {
				for name, open := range overlays {
					m := renderedDashboard(w, h)
					m.setTab(tb)
					m.setFocus(focus)
					open(m)

					// The layout must fill the screen on its own, not be
					// padded into it by View's final clamp.
					if got := len(strings.Split(m.frame(), "\n")); got != h {
						t.Fatalf("%dx%d tab %v focus %v modal %s: frame has %d lines, want %d", w, h, tb, focus, name, got, h)
					}
					lines := strings.Split(m.View().Content, "\n")
					if len(lines) != h {
						t.Fatalf("%dx%d tab %v focus %v modal %s: view has %d lines, want %d", w, h, tb, focus, name, len(lines), h)
					}
					for i, line := range lines {
						if got := lipgloss.Width(line); got > w {
							t.Fatalf("%dx%d tab %v focus %v modal %s: line %d is %d wide, want <= %d", w, h, tb, focus, name, i, got, w)
						}
					}
				}
			}
		}
	}
}

func TestHelpBarTruncatesToWidth(t *testing.T) {
	for _, w := range []int{60, 79, 80, 100, 120, 200} {
		for _, section := range allTabs {
			m := renderedDashboard(w, 40)
			m.setTab(section)

			bar := m.footer(m.lay)
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
		m.setTab(tabReading)
		if got := stripANSI(m.footer(m.lay)); !strings.Contains(got, "? help") {
			t.Fatalf("width %d: %q should always keep the help hint", w, got)
		}
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
	if got := searchOf(m).input.AvailableSuggestions(); len(got) != 0 {
		t.Fatalf("a fresh dashboard suggests %#v, want nothing until the user searches", got)
	}

	// A nil app must not panic on the way to the (skipped) save.
	if cmd := m.addRecentSearchQuery("east of eden"); cmd != nil {
		t.Fatal("addRecentSearchQuery() should not try to save without a store")
	}
	searchOf(m).updateSuggestions()

	got := searchOf(m).input.AvailableSuggestions()
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
	m.setTab(tabReading)

	cmd := send(t, m, runeKey('t'))
	if cmd == nil {
		t.Fatal("t in the Reading list should start a timer for the selection")
	}
	if !m.isLoading() {
		t.Fatal("starting a timer should be counted as in flight")
	}
	if timerPickerOf(m) != nil {
		t.Fatal("the Reading list already has a selection: no picker")
	}

	// With a timer running, t stops it.
	running := renderedDashboard(100, 40)
	running.setTab(tabReading)
	running.shared.timer = &model.TimerState{BookID: 1, StartedAt: time.Now()}

	if cmd := send(t, running, runeKey('t')); cmd == nil {
		t.Fatal("t should stop the running timer")
	}
	if timerPickerOf(running) != nil {
		t.Fatal("stopping a timer must not open the picker")
	}
}

// TestTimerKeyInOkuOpensThePicker: t over a list that is not the Reading one
// used to answer with a toast telling the reader to go and press it
// somewhere else. It opens the picker instead, on the book the cursor was
// over when that book is one of the ones being read, and on the Reading
// list's own selection otherwise.
func TestTimerKeyInOkuOpensThePicker(t *testing.T) {
	books := []model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune"}},
		{Book: model.Book{ID: 2, Title: "Foundation"}},
		{Book: model.Book{ID: 3, Title: "Meditations"}},
	}

	// The Oku selection is not a book being read: the picker opens on the
	// Reading list's cursor.
	m := renderedDashboard(100, 40)
	setLibrary(m, books, []model.UserBook{{Book: model.Book{ID: 9, Title: "Ulysses"}}})
	readingSection(m).list.Select(2)
	m.setTab(tabOku)

	send(t, m, runeKey('t'))
	if m.isLoading() {
		t.Fatal("t should open the picker, not start a timer straight away")
	}
	picker := timerPickerOf(m)
	if picker == nil {
		t.Fatalf("top modal = %T, want the book picker", m.topModal())
	}
	if picker.idx != 2 {
		t.Fatalf("picker opened on %d, want the Reading list's own selection (2)", picker.idx)
	}

	// The Oku selection is a book that is also being read — the shelves come
	// from a cache that can lag a status change — so the picker opens on it.
	m = renderedDashboard(100, 40)
	setLibrary(m, books, []model.UserBook{{Book: model.Book{ID: 2, Title: "Foundation"}}})
	m.setTab(tabOku)

	send(t, m, runeKey('t'))
	if picker = timerPickerOf(m); picker == nil {
		t.Fatalf("top modal = %T, want the book picker", m.topModal())
	}
	if picker.idx != 1 {
		t.Fatalf("picker opened on %d, want the book the cursor was over (1)", picker.idx)
	}

	// Enter starts a timer for it.
	send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.isLoading() {
		t.Fatal("Enter over the picker should start the timer")
	}
}

func TestEnterDoesNotChangeStatus(t *testing.T) {
	for _, tb := range []tab{tabReading, tabOku} {
		m := renderedDashboard(100, 40)
		m.setTab(tb)
		before := m.shared.reading[0].StatusID

		send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.isLoading() {
			t.Fatalf("tab %v: Enter started an operation", tb)
		}
		if m.toast.level == toastError {
			t.Fatalf("tab %v: toast = %+v, want no error", tb, m.toast)
		}
		if m.shared.reading[0].StatusID != before {
			t.Fatalf("tab %v: Enter changed the status to %v", tb, m.shared.reading[0].StatusID)
		}
		if m.focus != focusDetail {
			t.Fatalf("tab %v: Enter should move the keyboard into the detail pane, focus = %v", tb, m.focus)
		}

		// Esc gives it back.
		send(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
		if m.focus != focusContent {
			t.Fatalf("tab %v: Esc should return to the list, focus = %v", tb, m.focus)
		}
	}
}

func TestPageModalShowsTitleAndKeepsFormatHint(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setTab(tabReading)

	send(t, m, runeKey('u'))

	page := pageModalOf(m)
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
		if help.vp.TotalLineCount() <= help.vp.Height() {
			t.Fatalf("height %d: the help body should be taller than the window", h)
		}
		if !strings.Contains(help.View(m.lay, m.st), "j/k scroll") {
			t.Fatalf("height %d: an overflowing help modal should say it scrolls", h)
		}

		m.Update(runeKey('j'))
		if help.vp.YOffset() != 1 {
			t.Fatalf("height %d: j should scroll the body, YOffset = %d", h, help.vp.YOffset())
		}

		m.Update(runeKey('k'))
		if got := help.vp.YOffset(); got != 0 {
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
		m.setTab(tabReading)
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
	send(t, escaped, tea.KeyPressMsg{Code: tea.KeyEsc})
	if escaped.topModal() != nil || escaped.isLoading() {
		t.Fatal("esc should drop the change")
	}
}

func TestDidNotFinishAsksToo(t *testing.T) {
	m := renderedDashboard(100, 40)
	m.setTab(tabReading)

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
	m.setTab(tabReading)

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
			m.setTab(tabReading)
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

func TestContentPaneShowsHowFarDownTheListIs(t *testing.T) {
	books := make([]model.UserBook, 0, 30)
	for i := 0; i < 30; i++ {
		books = append(books, model.UserBook{Book: model.Book{ID: 100 + i, Title: fmt.Sprintf("Book %d", i)}})
	}

	m := renderedDashboard(120, 40)
	setLibrary(m, m.shared.reading, books)
	m.setTab(tabOku)

	badge := okuSection(m).overflowBadge()
	if badge != "1/30" {
		t.Fatalf("overflow badge = %q, want 1/30", badge)
	}
	// The badge sits on the pane's last inner row, over the rows the list
	// pads out to its full width.
	rows := strings.Split(stripANSI(m.frame()), "\n")
	last := rows[len(rows)-3]
	if !strings.Contains(last, badge) {
		t.Fatalf("the content pane's last row should carry the badge, got %q", last)
	}

	// A list that fits says nothing.
	setLibrary(m, m.shared.reading, books[:1])
	if got := okuSection(m).overflowBadge(); got != "" {
		t.Fatalf("overflow badge = %q, want none when the list fits", got)
	}
	if strings.Contains(stripANSI(m.frame()), "1/1") {
		t.Fatal("a list that fits should not be badged")
	}

	small := renderedDashboard(80, 24)
	setLibrary(small, books, small.shared.oku)
	small.setTab(tabReading)
	if got := readingSection(small).overflowBadge(); got != "1/30" {
		t.Fatalf("narrow overflow badge = %q, want 1/30", got)
	}
	if !strings.Contains(stripANSI(small.frame()), "1/30") {
		t.Fatal("the narrow Reading pane should show the overflow badge")
	}
}

func TestProgressRowFitsTheDetailPane(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {200, 50}} {
		m := renderedDashboard(size[0], size[1])
		m.setTab(tabReading)
		m.setFocus(focusDetail)

		w := m.lay.DetailInner
		detail := renderUserBook(*m.section().Selected().Book, m.shared.sessions, m.shared.now(), w, m.shared.density, m.st)
		for _, line := range strings.Split(stripANSI(detail), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Fatalf("%dx%d: detail line %q is %d wide, want <= %d", size[0], size[1], line, got, w)
			}
		}
		if !strings.Contains(stripANSI(m.frame()), "p.120 / 412 · 29%") {
			t.Fatalf("%dx%d: the progress row should be on screen in full:\n%s", size[0], size[1], stripANSI(m.frame()))
		}
	}
}

func TestTimerSectionStopsWithT(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setTab(tabTimer)
	m.shared.timer = &model.TimerState{BookID: 1, StartedAt: time.Now()}

	if cmd := send(t, m, runeKey('t')); cmd == nil {
		t.Fatal("t should stop the running timer in the Timer section too")
	}
	if timerPickerOf(m) != nil {
		t.Fatal("stopping must not open the book picker")
	}
}

func TestSecondTimerPressIsGuardedWhileInFlight(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setTab(tabReading)

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

func TestThemeResolvesDistinctColoursForLightAndDark(t *testing.T) {
	dark, light := NewTheme(true), NewTheme(false)
	colours := map[string][2]color.Color{
		"accent": {light.Accent, dark.Accent}, "heading": {light.Heading, dark.Heading},
		"text": {light.Text, dark.Text}, "textMuted": {light.TextMuted, dark.TextMuted},
		"textDim": {light.TextDim, dark.TextDim}, "border": {light.Border, dark.Border},
		"borderFocused": {light.BorderFocused, dark.BorderFocused},
		"surface":       {light.Surface, dark.Surface}, "success": {light.Success, dark.Success},
		"warning": {light.Warning, dark.Warning}, "error": {light.Error, dark.Error},
		"heat1": {light.Heat1, dark.Heat1}, "heat2": {light.Heat2, dark.Heat2},
		"heat3": {light.Heat3, dark.Heat3}, "heat4": {light.Heat4, dark.Heat4},
	}
	for name, pair := range colours {
		l, d := pair[0], pair[1]
		if l == nil || d == nil {
			t.Fatalf("%s: both sides of the theme must be set, got light=%v dark=%v", name, l, d)
		}
		if l == d {
			t.Fatalf("%s: light and dark are the same value %v", name, l)
		}
		if lRender, dRender := renderColour(l), renderColour(d); lRender == dRender {
			t.Fatalf("%s: renders the same on a light and a dark terminal: %q", name, dRender)
		}
	}

	// The heat ramp has four distinct steps on each side.
	for _, side := range []struct {
		name string
		th   Theme
	}{{"dark", dark}, {"light", light}} {
		seen := map[string]bool{}
		for _, c := range []color.Color{side.th.Heat1, side.th.Heat2, side.th.Heat3, side.th.Heat4} {
			r := renderColour(c)
			if seen[r] {
				t.Fatalf("%s heat ramp repeats %q", side.name, r)
			}
			seen[r] = true
		}
	}
}

// renderColour is one colour as the terminal would receive it, so two
// palette entries can be compared by what they actually draw.
func renderColour(c color.Color) string {
	return lipgloss.NewStyle().Foreground(c).Render("x")
}

func TestApplyThemeSetting(t *testing.T) {
	t.Cleanup(func() { _ = ApplyThemeSetting("auto") })

	if err := ApplyThemeSetting("light"); err != nil {
		t.Fatalf("ApplyThemeSetting(light) error = %v", err)
	}
	if isDark, pinned := PinnedDark(); !pinned || isDark {
		t.Fatalf("theme = light should pin a light background, got isDark=%v pinned=%v", isDark, pinned)
	}
	if err := ApplyThemeSetting("Dark"); err != nil {
		t.Fatalf("ApplyThemeSetting(Dark) error = %v", err)
	}
	if isDark, pinned := PinnedDark(); !pinned || !isDark {
		t.Fatalf("theme = dark should pin a dark background, got isDark=%v pinned=%v", isDark, pinned)
	}
	for _, ok := range []string{"", "auto", " AUTO "} {
		if err := ApplyThemeSetting(ok); err != nil {
			t.Fatalf("ApplyThemeSetting(%q) error = %v, want none", ok, err)
		}
		if _, pinned := PinnedDark(); pinned {
			t.Fatalf("theme = %q should leave the background to be detected", ok)
		}
	}
	if err := ApplyThemeSetting("solarized"); err == nil {
		t.Fatal("an unknown theme should be reported, not ignored")
	}
}

// TestBackgroundColourRebuildsTheStyles is the v2 replacement for lipgloss's
// adaptive colour: the terminal reports its background and the whole palette
// is rebuilt, list delegates and memoised pages included.
func TestBackgroundColourRebuildsTheStyles(t *testing.T) {
	m := renderedDashboard(120, 40)
	before := m.View().Content
	if !m.isDark {
		t.Fatal("the dashboard should start dark, before the terminal answers")
	}

	m.Update(backgroundMsg(false))
	if m.isDark {
		t.Fatal("a light background should have been applied")
	}
	if after := m.View().Content; after == before {
		t.Fatal("the frame renders the same after the palette was rebuilt for a light terminal")
	}
	if got, want := renderColour(m.st.th.Accent), renderColour(NewTheme(false).Accent); got != want {
		t.Fatalf("accent = %q after a light background, want %q", got, want)
	}
}

// TestPinnedThemeIgnoresTheTerminal: a `theme` config key is an answer, so
// the terminal is neither asked nor listened to.
func TestPinnedThemeIgnoresTheTerminal(t *testing.T) {
	if err := ApplyThemeSetting("dark"); err != nil {
		t.Fatalf("ApplyThemeSetting(dark) error = %v", err)
	}
	t.Cleanup(func() { _ = ApplyThemeSetting("auto") })

	m := renderedDashboard(120, 40)
	before := m.View().Content
	m.Update(backgroundMsg(false))
	if !m.isDark {
		t.Fatal("a pinned dark theme should survive a light background report")
	}
	if after := m.View().Content; after != before {
		t.Fatal("a pinned theme should not repaint on a background report")
	}
}

// keyMsgFor turns a binding's key name into the key press Bubble Tea would
// send for it. v2 keys are a code plus modifiers rather than a type, and
// String() is what the binding was written against, so the table is walked
// by name.
func keyMsgFor(t *testing.T, name string) tea.KeyPressMsg {
	t.Helper()
	for _, code := range []rune{
		tea.KeyEnter, tea.KeyEsc, tea.KeyTab,
		tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight,
		tea.KeyHome, tea.KeyEnd, tea.KeyPgUp, tea.KeyPgDown,
	} {
		for _, mod := range []tea.KeyMod{0, tea.ModShift, tea.ModCtrl} {
			msg := tea.KeyPressMsg{Mod: mod, Code: code}
			if msg.String() == name {
				return msg
			}
		}
	}
	for _, c := range "cdustg" {
		if msg := (tea.KeyPressMsg{Mod: tea.ModCtrl, Code: c}); msg.String() == name {
			return msg
		}
	}
	runes := []rune(name)
	if len(runes) != 1 {
		t.Fatalf("no key press for key %q", name)
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
		searchOf(m).results = []model.SearchResult{{ID: 7, Title: "Dune"}, {ID: 8, Title: "Dune Messiah"}, {ID: 9, Title: "Children of Dune"}}
		searchOf(m).rebuildResults()
		searchOf(m).list.Select(1)
		searchOf(m).input.SetValue("dune")
		return m
	}
	inSection := func(s tab) func() *Model {
		return func() *Model {
			m := base(120, 40)
			m.setTab(s)
			return m
		}
	}
	confirmWithCursor := func(cursor int) func() *Model {
		return func() *Model {
			m := inSection(tabReading)()
			send(t, m, runeKey('x'))
			m.topModal().(*confirmModal).c.Cursor = cursor
			return m
		}
	}

	// A detail pane with more in it than fits, so every scroll key has
	// somewhere to go — one variant at the top, one already scrolled.
	longReview := func(t tab, scrolled bool) func() *Model {
		return func() *Model {
			m := inSection(t)()
			books := m.shared.reading
			books[1].Review = strings.Repeat("A review long enough to scroll the detail pane. ", 60)
			setLibrary(m, books, m.shared.oku)
			m.sections[t].(*librarySection).list.Select(1)
			m.setFocus(focusDetail)
			if scrolled {
				m.frame() // fills the viewport, which is what there is to scroll
				m.detail.vp.ScrollDown(5)
			}
			return m
		}
	}

	states := []struct {
		name string
		// variants are arrangements of the same focus; a key passes when it
		// does something in any of them (a two-button dialog cannot move
		// left and right from one cursor).
		variants []func() *Model
		// selfKey is the tab number the state is already on. 1-5 are
		// advertised everywhere, and the one that lands where it already is
		// has nothing to do; it is asserted to be a no-op instead.
		selfKey string
	}{
		{name: "reading", variants: []func() *Model{inSection(tabReading)}, selfKey: "1"},
		{name: "oku", variants: []func() *Model{inSection(tabOku)}, selfKey: "2"},
		{name: "reading, detail focused", selfKey: "1",
			variants: []func() *Model{longReview(tabReading, false), longReview(tabReading, true)}},
		{name: "search input", variants: []func() *Model{func() *Model {
			// No selfKey: the input owns the keyboard, so 3 is a digit here
			// and the tab numbers are not advertised.
			m := withResults(inSection(tabSearch)())
			searchOf(m).focusInput()
			return m
		}}},
		{name: "search results", selfKey: "3", variants: []func() *Model{func() *Model {
			m := withResults(inSection(tabSearch)())
			searchOf(m).focusResults()
			return m
		}}},
		{name: "stats", selfKey: "4", variants: []func() *Model{func() *Model {
			// Short enough that the page overflows, one row down so it can
			// still scroll either way.
			m := base(80, 24)
			m.setTab(tabStats)
			m.frame() // fills the viewport, which is what there is to scroll
			statsOf(m).vp.ScrollDown(1)
			return m
		}}},
		{name: "timer, idle", variants: []func() *Model{inSection(tabTimer)}, selfKey: "5"},
		{name: "timer, picking a book", variants: []func() *Model{func() *Model {
			m := inSection(tabTimer)()
			m.push(newTimerPickerModal(m.shared, 1))
			return m
		}}},
		{name: "timer, running", selfKey: "5", variants: []func() *Model{func() *Model {
			m := inSection(tabTimer)()
			m.shared.timer = &model.TimerState{BookID: 1, StartedAt: time.Now()}
			return m
		}}},
		{name: "help modal", variants: []func() *Model{func() *Model {
			// Short enough that the body scrolls, scrolled one row so every
			// direction has somewhere to go.
			m := base(80, 24)
			m.setTab(tabReading)
			m.openHelp()
			m.topModal().(*helpModal).vp.SetYOffset(1)
			return m
		}}},
		{name: "confirm", variants: []func() *Model{confirmWithCursor(1), confirmWithCursor(0)}},
		{name: "page prompt", variants: []func() *Model{func() *Model {
			m := inSection(tabReading)()
			m.push(newPageModal(m.shared, m.st, m.shared.reading[1]))
			return m
		}}},
		{name: "review modal", variants: []func() *Model{func() *Model {
			m := inSection(tabReading)()
			openReview(m, m.shared.reading[1]).rating.SetValue("3")
			return m
		}}},
		{name: "undo on offer", selfKey: "2", variants: []func() *Model{func() *Model {
			m := inSection(tabOku)()
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
					if name == state.selfKey {
						m := state.variants[0]()
						before := m.tab
						send(t, m, msg)
						if m.tab != before {
							t.Errorf("%s: %q left the tab it was already on", state.name, name)
						}
						continue
					}
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
	return newHelpModal(m.keysBehind, m.version, m.st).body()
}

// TestHelpModalListsEveryGroupWithTheActiveOnesFirst checks the modal still
// teaches the other sections' keys: from Reading it names the search keys,
// dimmed, after the groups that apply.
func TestHelpModalListsEveryGroupWithTheActiveOnesFirst(t *testing.T) {
	// Dimming is the difference between a live key and a dead one. A v2
	// style always writes its colour — the profile is applied on the way out
	// to the terminal — so the two differ here with no setup.
	m := renderedDashboard(120, 40)
	m.setTab(tabReading)
	body := stripANSI(helpBody(m))

	for _, want := range []string{"Actions", "Navigation", "General", "Confirm", "Review", "Timer", "Data",
		"cycle mode", "back to the input", "jump to a tab", "undo the last change", "stop timer"} {
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
	if strings.Contains(raw, m.st.modalKey.Width(12).Render("m")) {
		t.Fatal("a key the focus does not understand should not be drawn as live")
	}
}

// TestOverloadedKeysMatchTheirHelp pins the help-modal labels of keys that
// mean different things in different places to what the handler does there.
func TestOverloadedKeysMatchTheirHelp(t *testing.T) {
	overResults := func() *Model {
		m := renderedDashboard(120, 40)
		m.setTab(tabSearch)
		searchOf(m).results = []model.SearchResult{{ID: 1, Title: "Dune"}}
		searchOf(m).rebuildResults()
		searchOf(m).focusResults()
		return m
	}

	// Esc and i go back to the input over the results; h and l keep walking
	// the strip, as they do in every other tab.
	m := overResults()
	k := m.activeKeys()
	rows := stripANSI(helpBody(m))
	send(t, m, runeKey('i'))
	if searchOf(m).focus != inputFocused || m.tab != tabSearch {
		t.Fatalf("i over the results: focus=%v tab=%v, want the input", searchOf(m).focus, m.tab)
	}
	m = overResults()
	send(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if searchOf(m).focus != inputFocused {
		t.Fatalf("Esc over the results: focus=%v, want the input", searchOf(m).focus)
	}
	if !slices.Contains(k.PrevSection.Keys(), "h") {
		t.Fatal("h should still walk the strip over the results")
	}
	m = overResults()
	send(t, m, runeKey('h'))
	if m.tab != tabOku || searchOf(m).focus != resultsFocused {
		t.Fatalf("h over the results: tab=%v focus=%v, want the previous tab", m.tab, searchOf(m).focus)
	}
	if !strings.Contains(rows, "Esc/i") || !strings.Contains(rows, "h/l") {
		t.Fatalf("results help should say Esc/i goes back to the input and h/l walk the strip:\n%s", rows)
	}
	m = overResults()
	m.Update(tea.KeyPressMsg{Mod: tea.ModShift, Code: tea.KeyTab})
	if m.tab != tabOku {
		t.Fatalf("shift+tab over the results: section=%v, want the previous section", m.tab)
	}

	// j/k move the cursor in the list and scroll the detail pane, so the one
	// row the help gives them has to say which it is.
	list := renderedDashboard(120, 40)
	list.setTab(tabReading)
	if d := list.activeKeys().upDownDesc(); d != "navigate" {
		t.Fatalf("upDownDesc over the list = %q, want navigate", d)
	}
	send(t, list, tea.KeyPressMsg{Code: tea.KeyEnter})
	if list.focus != focusDetail {
		t.Fatal("Enter should focus the detail pane")
	}
	if d := list.activeKeys().upDownDesc(); d != "scroll" {
		t.Fatalf("upDownDesc over the detail pane = %q, want scroll", d)
	}
	if rows := stripANSI(helpBody(list)); !strings.Contains(rows, "j/k           scroll") {
		t.Fatalf("a focused detail pane should label j/k as scrolling:\n%s", rows)
	}
	// h comes back from the detail pane rather than moving a tab along.
	send(t, list, runeKey('h'))
	if list.focus != focusContent || list.tab != tabReading {
		t.Fatalf("h over the detail pane: focus=%v tab=%v, want back to the Reading list", list.focus, list.tab)
	}
}

func TestLaterToastDropsThePendingUndo(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setTab(tabReading)
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
		m.setTab(tabReading)
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
	bar := stripANSI(m.footer(m.lay))
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
		m.setTab(tabReading)
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
	if bar := stripANSI(m.footer(m.lay)); !strings.Contains(bar, "Page 130") || !strings.Contains(bar, "U undo") {
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
	m.setTab(tabSearch)
	searchOf(m).focusInput()

	m.Update(runeKey('U'))
	if searchOf(m).input.Value() != "U" {
		t.Fatalf("search input = %q, want the letter typed", searchOf(m).input.Value())
	}
	if m.undo == nil {
		t.Fatal("typing must not spend the undo")
	}
}

func TestFocusIsVisibleWithoutColour(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setTab(tabReading)

	content := ansi.Strip(m.frame())
	m.setFocus(focusDetail)
	detail := ansi.Strip(m.frame())
	if content == detail {
		t.Fatal("moving the focus should change the frame once colour is stripped")
	}

	// The thick border is on the content pane first and on the detail pane
	// after; the row under the pane titles has one of each either way.
	row := func(frame string) string { return strings.Split(frame, "\n")[2] }
	if !strings.HasPrefix(strings.TrimSpace(row(content)), "┃") {
		t.Fatalf("the focused content pane should carry the thick border:\n%s", content)
	}
	if strings.HasPrefix(strings.TrimSpace(row(detail)), "┃") {
		t.Fatalf("the blurred content pane should carry the thin border:\n%s", detail)
	}
	if !strings.Contains(row(detail), "┃") {
		t.Fatalf("the focused detail pane should carry the thick border:\n%s", detail)
	}

	// The active tab is marked in the strip, not only coloured.
	header := ansi.Strip(m.header(m.lay))
	if !strings.Contains(header, "▸1 Reading") {
		t.Fatalf("the active tab should be marked in the strip: %q", header)
	}
	m.setTab(tabStats)
	if got := ansi.Strip(m.header(m.lay)); !strings.Contains(got, "▸4 Stats") || strings.Contains(got, "▸1 Reading") {
		t.Fatalf("the marker should follow the tab: %q", got)
	}
}

func TestSharedClockDrivesEveryRender(t *testing.T) {
	fixed := time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local)
	m := renderedDashboard(120, 40)
	m.shared.now = func() time.Time { return fixed }
	m.shared.timer = &model.TimerState{BookID: 1, StartedAt: fixed.Add(-90 * time.Second)}
	m.shared.timerBook = &m.shared.reading[0].Book
	m.shared.stats, m.shared.sessions = demoLocalData(fixed)

	m.setTab(tabTimer)
	frame := stripANSI(m.frame())
	if !strings.Contains(frame, "00:01:30") {
		t.Fatalf("the timer should be read off the shared clock:\n%s", frame)
	}
	if !strings.Contains(frame, "Today") {
		t.Fatalf("sessions should be dated against the shared clock:\n%s", frame)
	}
	m.setTab(tabReading)
	if got := stripANSI(m.header(m.lay)); !strings.Contains(got, "▶ 00:01:30") {
		t.Fatalf("the header's timer should be read off the shared clock:\n%s", got)
	}
}
