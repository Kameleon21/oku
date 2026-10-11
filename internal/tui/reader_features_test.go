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
		if !fb.called("CachedBooks", status) || !fb.called("ListBooks", status, true) {
			t.Fatalf("calls = %v, want %s read from the cache and then refreshed", fb.calls, status)
		}
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

	if fb.called("ListBooks") {
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
	if fb.called("ListBooks", model.StatusPaused) {
		t.Fatalf("calls = %v, want no paused request on the reload", fb.calls)
	}
	if len(m.shared.paused) != 1 || m.shared.paused[0].BookID != 1 {
		t.Fatalf("paused = %v, want Dune", m.shared.paused)
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
