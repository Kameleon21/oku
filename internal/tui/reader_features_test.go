package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/exp/golden"
)

func TestJournalModalPreservesTextOnFailureAndMatchesToken(t *testing.T) {
	m := newGoldenModel(t, 80, 24, tabReading)
	send(t, m, runeKey('n'))
	n, ok := m.topModal().(*journalModal)
	if !ok {
		t.Fatal("n did not open editor")
	}
	n.text.SetValue("a personal note")
	_, cmd := n.Update(keyMsgFor(t, "ctrl+s"))
	r, ok := cmd().(reqJournal)
	if !ok || r.event != "note" || r.text != "a personal note" {
		t.Fatalf("wrong request %#v", r)
	}
	done, _ := n.Update(opDoneMsg{op: opJournal, seq: n.token + 1})
	if done || !n.submitting {
		t.Fatal("unrelated result changed draft")
	}
	done, _ = n.Update(opDoneMsg{op: opJournal, seq: n.token, err: errors.New("offline")})
	if done || n.submitting || n.text.Value() != "a personal note" || n.err != "offline" {
		t.Fatal("lost failed draft")
	}
	n.Update(keyMsgFor(t, "tab"))
	if n.event != "quote" {
		t.Fatal("quote toggle failed")
	}
	done, _ = n.Update(opDoneMsg{op: opJournal, seq: n.token})
	if !done {
		t.Fatal("successful save did not close")
	}
}
func TestPausedShelfAndResumeRequest(t *testing.T) {
	m := newGoldenModel(t, 80, 24, tabReading)
	paused := m.shared.reading[0]
	paused.StatusID = model.StatusPaused
	m.shared.paused = []model.UserBook{paused}
	send(t, m, runeKey('P'))
	s := readingSection(m)
	if !s.showPaused || !strings.Contains(s.Title(), "Paused") || s.selected().BookID != paused.BookID {
		t.Fatal("paused shelf unavailable")
	}
	cmd := s.handleKey(runeKey('g'))
	r, ok := cmd().(reqChangeStatus)
	if !ok || r.to != model.StatusCurrentlyReading || r.book.BookID != paused.BookID {
		t.Fatalf("wrong resume %#v", r)
	}
}

// TestLaunchRefreshesEveryShelf: opening the dashboard shows the cache and
// then refreshes reading, want-to-read and paused from Hardcover however
// fresh the cache is, so changes made on the website show up.
func TestLaunchRefreshesEveryShelf(t *testing.T) {
	m := newTestModel()
	fb := fakeOf(m)
	fb.online = true
	fb.add(dune, foundation, meditation)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drain(t, m, m.Init())

	for _, status := range []model.Status{model.StatusCurrentlyReading, model.StatusWantToRead, model.StatusPaused} {
		if !fb.called("CachedBooks", status) || !fb.calledRemote("ListBooks", status, true) {
			t.Fatalf("calls = %v, want %s read from the cache and then refreshed", fb.calls, status)
		}
	}
	if len(fb.remotes) != 3 {
		t.Fatalf("remote calls = %v, want exactly the three shelves", fb.remotes)
	}
	if m.isLoading() || m.toast.text != "" {
		t.Fatalf("after launch: inflight = %d, toast = %q; want settled and quiet", m.inflight, m.toast.text)
	}
}

func TestOfflineLaunchSkipsTheRefresh(t *testing.T) {
	m := newTestModel()
	fb := fakeOf(m)
	fb.add(dune)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	drain(t, m, m.Init())

	if fb.called("ListBooks") || len(fb.remotes) != 0 {
		t.Fatalf("calls = %v, want no refresh while offline", fb.calls)
	}
	if len(m.shared.reading) != 1 || m.toast.text != "" {
		t.Fatalf("reading = %v, toast = %q; want the cached shelf and no toast", m.shared.reading, m.toast.text)
	}
}

// TestReloadAfterWriteReadsPausedFromTheCache: the write already patched the
// cache, so the reload must not spend a request on the (usually empty)
// paused shelf.
func TestReloadAfterWriteReadsPausedFromTheCache(t *testing.T) {
	m, fb := loadedDashboard(t)
	op := changeStatusCmd(m.ctx, m.backend, 1, "Dune", dune.StatusID, model.StatusPaused)
	drain(t, m, send(t, m, request(reqRunOp{cmd: op})))

	if !fb.called("ChangeStatus") || !fb.called("CachedBooks", model.StatusPaused) {
		t.Fatalf("calls = %v, want the write and a reload of the paused shelf from the cache", fb.calls)
	}
	if fb.called("ListBooks", model.StatusPaused) || fb.calledRemote("ListBooks", model.StatusPaused) {
		t.Fatalf("remote calls = %v, want no paused request on the reload", fb.remotes)
	}
	if len(m.shared.paused) != 1 || m.shared.paused[0].BookID != 1 {
		t.Fatalf("paused = %v, want Dune", m.shared.paused)
	}
}

// TestBackgroundRefreshDoesNotRefuseWrites: the launch refresh turns the
// spinner, but a write pressed meanwhile goes ahead; work the reader started
// still holds the next write back.
func TestBackgroundRefreshDoesNotRefuseWrites(t *testing.T) {
	m, _ := loadedDashboard(t)
	refresh := m.refreshLibrary(false)
	if !m.isLoading() || m.busy() {
		t.Fatalf("inflight = %d, background = %d: want the spinner on and writes allowed", m.inflight, m.background)
	}

	m.setTab(tabReading)
	write := send(t, m, request(reqProgress{book: dune, delta: 1}))
	if m.toast.text == inFlightNotice {
		t.Fatal("a background refresh refused a write")
	}
	if !m.busy() {
		t.Fatal("the write the reader started should hold the next one back")
	}
	if send(t, m, request(reqProgress{book: dune, delta: 1})); m.toast.text != inFlightNotice {
		t.Fatalf("toast = %q, want the second write refused while the first runs", m.toast.text)
	}

	drain(t, m, tea.Batch(refresh, write))
	if m.inflight != 0 || m.background != 0 {
		t.Fatalf("inflight = %d, background = %d once everything landed, want 0", m.inflight, m.background)
	}
}

// TestOvertakenRefreshKeepsTheWrite: a refresh that fetched before a status
// change and lands after it must not put the book back. Its shelves are set
// aside and Hardcover, which has the change, is asked again.
func TestOvertakenRefreshKeepsTheWrite(t *testing.T) {
	m, fb := loadedDashboard(t)

	// The launch refresh fetches the shelves as they were...
	stale := refreshLibraryCmd(m.ctx, fb, m.writes, false)()
	m.inflight++
	m.background++

	// ...then Dune is paused before its result lands.
	op := changeStatusCmd(m.ctx, m.backend, 1, "Dune", dune.StatusID, model.StatusPaused)
	drain(t, m, send(t, m, request(reqRunOp{cmd: op})))

	fb.mu.Lock()
	fb.remotes = nil
	fb.mu.Unlock()
	drain(t, m, send(t, m, resultMsg{msg: stale, lane: laneBackground}))

	if !fb.calledRemote("ListBooks", model.StatusCurrentlyReading, true) {
		t.Fatalf("remote calls = %v, want the shelves fetched again", fb.remotes)
	}
	for _, b := range m.shared.reading {
		if b.BookID == 1 {
			t.Fatal("the overtaken refresh put Dune back on the Reading shelf")
		}
	}
	if len(m.shared.paused) != 1 || m.shared.paused[0].BookID != 1 {
		t.Fatalf("paused on screen = %v, want Dune", m.shared.paused)
	}
	if paused := fb.shelf(model.StatusPaused); len(paused) != 1 || paused[0].BookID != 1 {
		t.Fatalf("paused in the cache = %v, want Dune", paused)
	}
	if !m.dirty || m.inflight != 0 || m.background != 0 {
		t.Fatalf("dirty = %v, inflight = %d, background = %d; want dirty and settled", m.dirty, m.inflight, m.background)
	}
}

// TestRefreshLandingBeforeTheWriteKeepsTheWrite is the narrow window: a
// refresh fetches, a status change starts, the refresh's cache replace
// clobbers the change's patch, and the refresh lands while the change is
// still running, with no write finished since it was stamped. Neither its
// shelves nor the clobbered cache may win: once the change lands the
// shelves are fetched again from Hardcover.
func TestRefreshLandingBeforeTheWriteKeepsTheWrite(t *testing.T) {
	m, fb := loadedDashboard(t)
	ghost := model.UserBook{BookID: 99, StatusID: model.StatusCurrentlyReading, Book: model.Book{ID: 99, Title: "Ghost"}}

	stale := refreshLibraryCmd(m.ctx, fb, m.writes, false)().(libraryLoadedMsg)
	stale.reading = append(stale.reading, ghost)
	m.inflight++
	m.background++
	before := map[model.Status][]model.UserBook{}
	for status, books := range fb.books {
		before[status] = copyShelf(books)
	}

	op := changeStatusCmd(m.ctx, m.backend, 1, "Dune", dune.StatusID, model.StatusPaused)
	send(t, m, request(reqRunOp{cmd: op}))

	if cmd := send(t, m, resultMsg{msg: stale, lane: laneBackground}); cmd != nil {
		t.Fatal("the retry should wait for the running write")
	}
	for _, b := range m.shared.reading {
		if b.BookID == ghost.BookID {
			t.Fatal("a refresh that landed during a write was applied")
		}
	}

	// The change reaches Hardcover and patches the cache; the refresh's
	// replace then puts the old shelves back in the cache.
	done := op()
	fb.mu.Lock()
	fb.hardcover = map[model.Status][]model.UserBook{}
	for status, books := range fb.books {
		fb.hardcover[status] = copyShelf(books)
	}
	fb.books = before
	fb.remotes = nil
	fb.mu.Unlock()

	drain(t, m, send(t, m, resultMsg{msg: done, lane: laneWrite}))

	if !fb.calledRemote("ListBooks", model.StatusPaused, true) {
		t.Fatalf("remote calls = %v, want the shelves fetched again", fb.remotes)
	}
	for _, b := range m.shared.reading {
		if b.BookID == 1 || b.BookID == ghost.BookID {
			t.Fatalf("reading on screen = %v, want neither Dune nor the stale rows", m.shared.reading)
		}
	}
	if len(m.shared.paused) != 1 || m.shared.paused[0].BookID != 1 {
		t.Fatalf("paused on screen = %v, want Dune", m.shared.paused)
	}
	if paused := fb.shelf(model.StatusPaused); len(paused) != 1 || paused[0].BookID != 1 {
		t.Fatalf("paused in the cache = %v, want Dune back after the clobber", paused)
	}
	if m.inflight != 0 || m.writing != 0 || m.background != 0 {
		t.Fatalf("inflight = %d, writing = %d, background = %d; want settled", m.inflight, m.writing, m.background)
	}
}

// TestOvertakenRefreshWaitsOffline: with Hardcover out of reach the retry
// is left to the reconcile rather than failing with a toast.
func TestOvertakenRefreshWaitsOffline(t *testing.T) {
	m, fb := loadedDashboard(t)
	stale := refreshLibraryCmd(m.ctx, fb, m.writes, false)()
	m.inflight++
	m.background++
	drain(t, m, send(t, m, request(reqRunOp{cmd: changeStatusCmd(m.ctx, m.backend, 1, "Dune", dune.StatusID, model.StatusPaused)})))

	fb.online = false
	m.toast = toast{}
	if cmd := send(t, m, resultMsg{msg: stale, lane: laneBackground}); cmd != nil {
		t.Fatal("an overtaken refresh should not retry offline")
	}
	if !m.dirty || m.toast.text != "" || m.inflight != 0 {
		t.Fatalf("dirty = %v, toast = %q, inflight = %d; want dirty, quiet and settled", m.dirty, m.toast.text, m.inflight)
	}
}

func TestTimerCommandsTakeTheBookFromTheBackend(t *testing.T) {
	fb := newFakeBackend()
	fb.add(model.UserBook{StatusID: model.StatusCurrentlyReading, Book: model.Book{ID: 1, Title: "Dune"}})

	if msg := startTimerForBookCmd(fb, 1)().(timerOpDoneMsg); msg.err != nil || msg.info != "Timer started — Dune" {
		t.Fatalf("start = %+v, want the book's title", msg)
	}
	local := loadLocalDataCmd(fb, time.Now)().(localDataLoadedMsg)
	if local.timerState == nil || local.timerState.BookID != 1 || local.timerBook == nil || local.timerBook.Title != "Dune" {
		t.Fatalf("local load timer = %+v / %+v, want Dune's timer", local.timerState, local.timerBook)
	}

	fb.timer = nil
	if msg := startTimerForBookCmd(fb, 99)().(timerOpDoneMsg); msg.err != nil || msg.info != "Timer started" {
		t.Fatalf("start for an uncached book = %+v, want the plain notice", msg)
	}
	if local := loadLocalDataCmd(fb, time.Now)().(localDataLoadedMsg); local.timerState == nil || local.timerBook != nil {
		t.Fatalf("local load timer = %+v / %+v, want a timer with no book", local.timerState, local.timerBook)
	}
}

func TestQueueMoveKeepsSelection(t *testing.T) {
	m := newGoldenModel(t, 80, 24, tabOku)
	s := okuSection(m)
	books := s.books()
	first, second := books[0].BookID, books[1].BookID
	m.handleReaderRequest(queueLoadedMsg{ids: []int{second, first}, bookID: first})
	if s.books()[0].BookID != second || s.selected().BookID != first {
		t.Fatal("ranking lost order or selection")
	}
}
func TestTrendingRejectsStaleSearchResult(t *testing.T) {
	m := newGoldenModel(t, 80, 24, tabSearch)
	s := searchOf(m)
	s.focus = resultsFocused
	cmd := s.handleKey(runeKey('D'))
	r, ok := cmd().(reqTrending)
	if !ok {
		t.Fatal("D did not request trending")
	}
	s.seq++
	s.applyLoaded(searchLoadedMsg{seq: r.seq, results: []model.SearchResult{{ID: 99, Title: "stale"}}})
	for _, b := range s.results {
		if b.ID == 99 {
			t.Fatal("stale discovery overwrote current search")
		}
	}
}
func TestRichDetailRendersAndScrolls(t *testing.T) {
	m := newGoldenModel(t, 80, 24, tabReading)
	id := m.shared.reading[0].BookID
	m.handleReaderRequest(detailLoadedMsg{id: id, book: &model.BookDetail{Description: strings.Repeat("Long description. ", 100), EditionsCount: 5, RatingsDistribution: `{"5":10,"4":5}`}})
	m.setFocus(focusDetail)
	m.detail.View(m.section().Selected(), m.tab)
	if m.detail.vp.TotalLineCount() <= m.detail.vp.Height() {
		t.Fatal("rich detail is not scrollable")
	}
	body := renderRichDetail(m.shared.details[id], 30, m.st)
	if !strings.Contains(body, "Editions") || !strings.Contains(body, "Community ratings") {
		t.Fatal(body)
	}
}
func TestGoldenReaderFeatures(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"40x16", 40, 16}} {
		t.Run(size.name, func(t *testing.T) {
			m := newGoldenModel(t, size.w, size.h, tabReading)
			send(t, m, runeKey('n'))
			n := m.topModal().(*journalModal)
			n.text.SetValue("A note about this book.")
			n.text.Blur()
			golden.RequireEqual(t, []byte(frameAt(m, colorprofile.NoTTY)))
		})
	}
}
func TestJournalEditorKeysStayText(t *testing.T) {
	m := newGoldenModel(t, 80, 24, tabReading)
	send(t, m, runeKey('n'))
	n := m.topModal().(*journalModal)
	send(t, m, tea.KeyPressMsg{Code: 'p', Text: "p"})
	if n.text.Value() != "p" {
		t.Fatal("editor swallowed a status letter")
	}
}
