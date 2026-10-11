package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
)

// The tests here run real commands: a key or a request starts work against
// the fake backend, drain executes it and feeds the results back in, and
// the assertions are on what the dashboard shows afterwards — the toast,
// the undo, the reload, and above all the in-flight count, which every
// command must leave where it found it.

var (
	dune       = model.UserBook{ID: 11, BookID: 1, StatusID: model.StatusCurrentlyReading, CurrentPage: 120, Book: model.Book{ID: 1, Title: "Dune", Pages: 412}}
	foundation = model.UserBook{ID: 12, BookID: 2, StatusID: model.StatusCurrentlyReading, Book: model.Book{ID: 2, Title: "Foundation", Pages: 255}}
	meditation = model.UserBook{ID: 13, BookID: 3, StatusID: model.StatusWantToRead, Book: model.Book{ID: 3, Title: "Meditations"}}
)

// loadedDashboard is a dashboard over an online fake with two books on the
// Reading shelf and one on Oku, loaded the way the program loads it: Init's
// commands run against the fake and their results land through the runner.
func loadedDashboard(t *testing.T) (*Model, *fakeBackend) {
	t.Helper()
	m := newTestModel()
	fb := fakeOf(m)
	fb.online = true
	fb.add(dune, foundation, meditation)
	fb.queue = []int{3}
	fb.details[1] = &api.BookDetail{Description: "A desert planet."}
	fb.journals[1] = []api.JournalEntry{{Event: "note", Entry: "The spice must flow."}}
	fb.results = []model.SearchResult{{ID: 9, Title: "Ubik"}}
	fb.trending = []model.SearchResult{{ID: 8, Title: "Hyperion"}}
	m.shared.now = func() time.Time { return fixedNow }
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	drain(t, m, m.Init())
	if !m.shared.loaded || !m.shared.localLoaded {
		t.Fatalf("loaded = %v, localLoaded = %v: Init's loads should have landed", m.shared.loaded, m.shared.localLoaded)
	}
	if m.isLoading() {
		t.Fatalf("inflight = %d after Init's loads landed, want 0", m.inflight)
	}
	if len(m.shared.reading) != 2 || len(m.shared.oku) != 1 {
		t.Fatalf("reading = %d, oku = %d, want the fake's shelves", len(m.shared.reading), len(m.shared.oku))
	}
	fb.calls = nil
	return m, fb
}

func TestStatusChangeRunsThroughToTheToastAndReload(t *testing.T) {
	m, fb := loadedDashboard(t)
	m.setTab(tabReading)

	drain(t, m, send(t, m, runeKey('f')))

	if !fb.called("ChangeStatus", 1, model.StatusRead) {
		t.Fatalf("calls = %v, want the status change sent", fb.calls)
	}
	if m.toast.level != toastSuccess || m.toast.text != "Moved 'Dune' to Read" {
		t.Fatalf("toast = %+v, want the move reported", m.toast)
	}
	if m.undo == nil || m.undo.op != opStatus || m.undo.bookID != 1 || m.undo.toStatus != model.StatusCurrentlyReading {
		t.Fatalf("undo = %+v, want the way back to Reading", m.undo)
	}
	if !m.dirty {
		t.Fatal("a change that reached Hardcover should mark the library dirty")
	}
	if !fb.called("ListBooks", model.StatusCurrentlyReading, false) || !fb.called("GetReadingStats") {
		t.Fatalf("calls = %v, want the library and local data reloaded", fb.calls)
	}
	if len(m.shared.reading) != 1 || m.shared.reading[0].Book.Title != "Foundation" {
		t.Fatalf("reading = %v, want Dune gone from the shelf after the reload", m.shared.reading)
	}
	if m.isLoading() {
		t.Fatalf("inflight = %d after the change and its reloads, want 0", m.inflight)
	}
}

func TestStatusChangeWithStaleCacheWarnsAndStillReloads(t *testing.T) {
	m, fb := loadedDashboard(t)
	m.setTab(tabReading)
	fb.errs["ChangeStatus"] = app.ErrCacheRefresh

	drain(t, m, send(t, m, runeKey('f')))

	if m.toast.level != toastWarn || m.toast.text != "Moved 'Dune' to Read; "+cacheWarning {
		t.Fatalf("toast = %+v, want the move with the cache warning", m.toast)
	}
	if m.undo == nil || m.undo.op != opStatus {
		t.Fatal("the change stands on Hardcover, so undo should be on offer")
	}
	if !m.dirty || !fb.called("ListBooks", model.StatusCurrentlyReading, false) {
		t.Fatalf("dirty = %v, calls = %v: a change Hardcover took is reloaded and marked dirty", m.dirty, fb.calls)
	}
	if m.isLoading() {
		t.Fatalf("inflight = %d, want 0", m.inflight)
	}
}

func TestFailedCommandClearsLoadingAndDoesNotRefuseTheNextSearch(t *testing.T) {
	m, fb := loadedDashboard(t)
	m.setTab(tabReading)
	fb.errs["ChangeStatus"] = errors.New("boom")

	drain(t, m, send(t, m, runeKey('f')))

	if m.toast.level != toastError || m.toast.text != "boom" {
		t.Fatalf("toast = %+v, want the failure reported", m.toast)
	}
	if m.undo != nil || m.dirty {
		t.Fatalf("undo = %v, dirty = %v: a failed change changed nothing", m.undo, m.dirty)
	}
	if fb.called("ListBooks") {
		t.Fatalf("calls = %v, want no reload after a failure", fb.calls)
	}
	if m.isLoading() {
		t.Fatalf("inflight = %d after the failure, want 0", m.inflight)
	}

	// The failure released its slot: the next piece of work is not refused.
	drain(t, m, send(t, m, runeKey('+')))
	if m.toast.text == inFlightNotice {
		t.Fatal("+ after a failed change was refused as if the change were still running")
	}
	if !fb.called("UpdateProgress", 1) {
		t.Fatalf("calls = %v, want the progress update sent", fb.calls)
	}

	m.setTab(tabSearch)
	searchOf(m).focusInput()
	searchOf(m).input.SetValue("ubik")
	cmd := send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !searchOf(m).loading {
		t.Fatal("the search after a failure should start")
	}
	drain(t, m, cmd)
	if !fb.called("SearchBooks", "ubik") || len(searchOf(m).results) != 1 {
		t.Fatalf("calls = %v, results = %d: the search should have run and landed", fb.calls, len(searchOf(m).results))
	}
	if m.isLoading() {
		t.Fatalf("inflight = %d after the search, want 0", m.inflight)
	}
}

func TestOfflineRequestsWarnWithoutStartingWork(t *testing.T) {
	warned := func(t *testing.T, m *Model, fb *fakeBackend, what string) {
		t.Helper()
		if m.toast.level != toastWarn || m.toast.text != "Hardcover is unavailable" {
			t.Fatalf("%s: toast = %+v, want the offline warning", what, m.toast)
		}
		if m.isLoading() {
			t.Fatalf("%s: inflight = %d, want nothing started", what, m.inflight)
		}
		if len(fb.calls) != 0 {
			t.Fatalf("%s: calls = %v, want the backend left alone", what, fb.calls)
		}
	}

	m, fb := loadedDashboard(t)
	fb.online = false
	m.setTab(tabReading)
	drain(t, m, send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}))
	warned(t, m, fb, "detail")
	if m.focus != focusDetail {
		t.Fatal("Enter should still move the keyboard to the detail pane")
	}

	m, fb = loadedDashboard(t)
	fb.online = false
	m.setTab(tabOku)
	drain(t, m, send(t, m, runeKey('K')))
	warned(t, m, fb, "queue move")
	drain(t, m, send(t, m, runeKey('R')))
	warned(t, m, fb, "queue refresh")

	m, fb = loadedDashboard(t)
	fb.online = false
	drain(t, m, send(t, m, request(reqPausedShelf{})))
	warned(t, m, fb, "paused shelf")

	// The journal modal covers the status bar, so its refusal is its own
	// failed result, reported inside the panel and as an error toast behind
	// it; nothing is started either way.
	m, fb = loadedDashboard(t)
	fb.online = false
	m.setTab(tabReading)
	send(t, m, runeKey('n'))
	n, ok := m.topModal().(*journalModal)
	if !ok {
		t.Fatalf("top modal = %T, want the journal modal", m.topModal())
	}
	n.text.SetValue("Spice.")
	drain(t, m, send(t, m, tea.KeyPressMsg{Mod: tea.ModCtrl, Code: 's'}))
	if m.topModal() != n || n.submitting || n.err != "Hardcover is unavailable" {
		t.Fatalf("modal = %T, submitting = %v, err = %q: want the refusal shown inside the open modal", m.topModal(), n.submitting, n.err)
	}
	if m.toast.level != toastError || m.toast.text != "Hardcover is unavailable" {
		t.Fatalf("toast = %+v, want the failure behind the modal", m.toast)
	}
	if m.isLoading() || len(fb.calls) != 0 {
		t.Fatalf("inflight = %d, calls = %v: want nothing started", m.inflight, fb.calls)
	}
}

func TestDetailAndJournalLoadTogetherAndLandSeparately(t *testing.T) {
	m, fb := loadedDashboard(t)
	m.setTab(tabReading)

	cmd := send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.inflight != 2 {
		t.Fatalf("inflight = %d, want the detail and the journal both counted", m.inflight)
	}
	drain(t, m, cmd)

	if !fb.called("GetBookDetail", 1) || !fb.called("BookJournal", 1) {
		t.Fatalf("calls = %v, want both loads sent", fb.calls)
	}
	if m.shared.details[1] == nil || len(m.shared.journals[1]) != 1 {
		t.Fatalf("details = %v, journals = %v: want both results applied", m.shared.details, m.shared.journals)
	}
	if m.isLoading() {
		t.Fatalf("inflight = %d after both landed, want 0", m.inflight)
	}
	if frame := stripANSI(m.frame()); !strings.Contains(frame, "A desert planet.") || !strings.Contains(frame, "The spice must flow.") {
		t.Fatalf("the detail pane should show both:\n%s", frame)
	}
}

// TestEveryCommandLeavesTheCountWhereItFoundIt is the runner's invariant:
// whatever a request starts, and however it ends — well, badly, or offline
// — the in-flight count is back where it was once the results have landed.
func TestEveryCommandLeavesTheCountWhereItFoundIt(t *testing.T) {
	requests := map[string]func(m *Model) any{
		"sync":            func(*Model) any { return reqSync{} },
		"refresh library": func(*Model) any { return reqRefresh{} },
		"refresh local":   func(*Model) any { return reqRefresh{local: true} },
		"progress":        func(*Model) any { return reqProgress{book: dune, delta: 10} },
		"set page":        func(*Model) any { return reqSetPage{bookID: 1, title: "Dune", prevPage: 120, raw: "130", token: 1} },
		"change status":   func(*Model) any { return reqChangeStatus{book: dune, to: model.StatusRead} },
		"review":          func(*Model) any { return reqReview{bookID: 1, rating: 4, review: "Fine.", token: 1} },
		"add from search": func(*Model) any {
			return reqAddFromSearch{result: model.SearchResult{ID: 9}, to: model.StatusWantToRead}
		},
		"search":        func(m *Model) any { return reqSearch{query: "ubik", mode: model.SearchModeBook, seq: searchOf(m).seq} },
		"trending":      func(m *Model) any { return reqTrending{seq: searchOf(m).seq} },
		"book detail":   func(*Model) any { return reqBookDetail{selection{Book: &dune}} },
		"result detail": func(*Model) any { return reqBookDetail{selection{Result: &model.SearchResult{ID: 1}}} },
		"paused shelf":  func(*Model) any { return reqPausedShelf{} },
		"queue move":    func(*Model) any { return reqQueueMove{3, 1} },
		"queue refresh": func(*Model) any { return reqQueueRefresh{} },
		"private note":  func(*Model) any { return reqPrivateNote{1, 1, "Mine."} },
		"journal":       func(*Model) any { return reqJournal{1, 1, "note", "Spice."} },
		"timer start":   func(*Model) any { return reqTimer{start: true, bookID: 1} },
		"timer stop":    func(*Model) any { return reqTimer{} },
		"timer toggle":  func(*Model) any { return reqTimerToggle{book: &dune, reading: true} },
		"confirmed op": func(m *Model) any {
			return reqRunOp{cmd: changeStatusCmd(m.ctx, m.backend, 1, "Dune", dune.StatusID, model.StatusIgnored)}
		},
	}
	arrange := map[string]func(fb *fakeBackend){
		"online":  func(*fakeBackend) {},
		"offline": func(fb *fakeBackend) { fb.online = false },
		"failing": func(fb *fakeBackend) {
			for _, method := range []string{"ListCachedBooks", "CachedBooks", "ListBooks", "ListAllCachedUserBooks",
				"CachedBook", "CachedQueueOrder", "SyncAll", "ChangeStatus", "UpdateProgress", "ReviewBook",
				"AddJournalEntry", "MoveQueueBook", "RefreshQueue", "SearchBooks", "TrendingBooks", "GetBookDetail",
				"BookJournal", "GetReadingStats", "ListPrivateNotes", "SavePrivateNote", "RecentSearches",
				"SaveRecentSearches", "TimerStart", "TimerStop", "TimerStatus", "TimerList"} {
				fb.errs[method] = errors.New("boom")
			}
		},
		"stale cache": func(fb *fakeBackend) {
			for _, method := range []string{"ChangeStatus", "UpdateProgress", "ReviewBook"} {
				fb.errs[method] = app.ErrCacheRefresh
			}
		},
	}

	for name, req := range requests {
		for state, prepare := range arrange {
			t.Run(name+"/"+state, func(t *testing.T) {
				m, fb := loadedDashboard(t)
				prepare(fb)
				before := m.inflight

				drain(t, m, send(t, m, request(req(m))))

				if m.inflight != before {
					t.Fatalf("inflight = %d, want %d once every result has landed", m.inflight, before)
				}
			})
		}
	}
}

func TestRunnerCountsABatchedCommandPerPartAndANilResultOnce(t *testing.T) {
	m := newTestModel()
	say := func(text string) tea.Cmd {
		return func() tea.Msg { return opDoneMsg{op: opSync, info: text} }
	}

	// A command that is itself a batch is counted once on the way out and
	// once per part once it answers.
	cmd := m.run(tea.Batch(say("one"), say("two")))
	if m.inflight != 1 {
		t.Fatalf("inflight = %d, want the batch counted once", m.inflight)
	}
	drain(t, m, cmd)
	if m.inflight != 0 {
		t.Fatalf("inflight = %d after both parts answered, want 0", m.inflight)
	}
	if m.toast.text != "one" && m.toast.text != "two" {
		t.Fatalf("toast = %+v, want one of the parts' results applied", m.toast)
	}

	// A command with nothing to say still releases its slot.
	drain(t, m, m.run(func() tea.Msg { return nil }))
	if m.inflight != 0 {
		t.Fatalf("inflight = %d after a nil result, want 0", m.inflight)
	}

	// Nothing to run is nothing.
	if m.run(nil) != nil || m.inflight != 0 {
		t.Fatalf("run(nil) = cmd, inflight = %d: want nothing started", m.inflight)
	}
}

// TestDrainTellsBatchesApart pins the helper the tests above lean on to
// the toolchain: a tea.Batch is recognised by name, and nothing else is.
func TestDrainTellsBatchesApart(t *testing.T) {
	a := func() tea.Msg { return nil }
	if !isBatch(tea.Batch(a, a)) {
		t.Fatal("a batch of two commands should be recognised")
	}
	for name, cmd := range map[string]tea.Cmd{
		"sequence": tea.Sequence(a, a),
		"tick":     tea.Tick(time.Hour, func(time.Time) tea.Msg { return nil }),
		"runner":   runCmd{a}.run,
		"request":  request(nil),
		"plain":    a,
	} {
		if isBatch(cmd) {
			t.Fatalf("%s was taken for a batch", name)
		}
	}
	if reflect.ValueOf(runCmd{a}.run).Pointer() != runFn {
		t.Fatal("the runner's commands should share one function")
	}
}
