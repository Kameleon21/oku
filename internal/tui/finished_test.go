package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/store"
	"github.com/charmbracelet/x/exp/golden"
)

func finishedFixture() []model.UserBook {
	var books []model.UserBook
	for i := 0; i < 16; i++ {
		date := time.Date(2026, 9, 20-i, 0, 0, 0, 0, time.UTC)
		books = append(books, model.UserBook{ID: i + 1, BookID: i + 1, StatusID: model.StatusRead, Rating: 4,
			Book:          model.Book{ID: i + 1, Title: fmt.Sprintf("Finished book %02d", i+1), Authors: []string{"Sample Author"}, Pages: 250},
			UserBookReads: []model.UserBookRead{{ID: 100 + i, UserBookID: i + 1, ProgressPages: 250, FinishedAt: &date}}})
	}
	return books
}
func finishedModel(t *testing.T, w, h int) *Model {
	m := newGoldenModel(t, w, h, tabStats)
	m.shared.stats.Finished = finishedFixture()
	m.shared.stats.Year.BooksFinished = len(m.shared.stats.Finished)
	send(t, m, runeKey('b'))
	return m
}
func TestFinishedBooksNavigationAndActions(t *testing.T) {
	for _, width := range []int{80, 120} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := finishedModel(t, width, 24)
			s := statsOf(m)
			if !s.browsing || m.lay.Split != (width >= 100) {
				t.Fatalf("browse layout: %+v", m.lay)
			}
			send(t, m, runeKey('j'))
			if got := s.Selected().Book.BookID; got != 2 {
				t.Fatalf("selected %d", got)
			}
			send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
			if m.focus != focusDetail || m.lay.DetailOnly != (width < 100) {
				t.Fatalf("detail layout: %+v", m.lay)
			}
			send(t, m, runeKey('n'))
			n, ok := m.topModal().(*journalModal)
			if !ok || !n.private || n.bookID != 2 {
				t.Fatalf("private note = %#v", m.topModal())
			}
			send(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
			send(t, m, runeKey('v'))
			r, ok := m.topModal().(*reviewModal)
			if !ok || r.book.BookID != 2 {
				t.Fatalf("review = %#v", m.topModal())
			}
			send(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
			send(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.focus != focusContent || !s.browsing {
				t.Fatal("Esc must return to list first")
			}
			send(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
			if s.browsing || m.lay.Split {
				t.Fatal("Esc must return to full-width stats")
			}
		})
	}
}
func TestFinishedBooksSelectionSurvivesReload(t *testing.T) {
	m := finishedModel(t, 120, 40)
	s := statsOf(m)
	s.finished.Select(4)
	selected := s.Selected().Book.UserBookReads[0].ID
	m.shared.stats.Finished = append(finishedFixture()[:1], m.shared.stats.Finished...)
	s.Update(dataChangedMsg{dataLocal})
	if got := s.Selected().Book.UserBookReads[0].ID; got != selected {
		t.Fatalf("selection changed to %d from %d", got, selected)
	}
	m.shared.stats.Finished = nil
	s.Update(dataChangedMsg{dataLocal})
	if s.Selected().Book != nil || m.activeKeys().Note.Enabled() || !strings.Contains(s.View(70, 20), "No completed reads") {
		t.Fatal("empty state must disable book actions")
	}
}
func TestPrivateNoteSavesWithoutHardcover(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "oku.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := finishedModel(t, 80, 24)
	m.app = &app.App{Store: db} // No API client: saving must remain fully local.
	send(t, m, runeKey('n'))
	n := m.topModal().(*journalModal)
	n.text.SetValue("My private thoughts")
	_, cmd := n.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	req, ok := cmd().(reqPrivateNote)
	if !ok {
		t.Fatal("private note attempted to use journal API")
	}
	// Execute the real request handler with no network client.
	m.spinning = true // omit the unrelated spinner command from this operation
	work, handled := m.handleReaderRequest(req)
	if !handled || work == nil {
		t.Fatal("private save request was not handled")
	}
	done, ok := work().(opDoneMsg)
	if !ok || done.err != nil || done.op != opPrivateNote {
		t.Fatalf("save result: %+v", done)
	}
	m.Update(done)
	notes, err := db.ListPrivateNotes()
	if err != nil || notes[req.bookID] != req.text {
		t.Fatalf("persisted notes: %v, %v", notes, err)
	}
	if m.topModal() != nil {
		t.Fatal("successful save did not close editor")
	}
	send(t, m, runeKey('n'))
	if got := m.topModal().(*journalModal).text.Value(); got != req.text {
		t.Fatalf("reopened note = %q", got)
	}
	m.detail.Resize(76, 20)
	if got := m.detail.render(statsOf(m).Selected(), tabStats); !strings.Contains(got, req.text) {
		t.Fatal("note missing from details")
	}
}
func TestGoldenFinishedBooks(t *testing.T) {
	for _, size := range [][2]int{{40, 16}, {80, 24}, {120, 40}} {
		for _, detail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d_detail_%t", size[0], size[1], detail), func(t *testing.T) {
				m := finishedModel(t, size[0], size[1])
				m.shared.privateNotes = map[int]string{1: "A book worth revisiting.\nKeep this thought private."}
				if detail {
					send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
				}
				golden.RequireEqual(t, []byte(frameAt(m, layoutProfile)))
			})
		}
	}
}

func TestFinishedRereadDetailsAndPaging(t *testing.T) {
	m := finishedModel(t, 120, 40)
	s := statsOf(m)
	first := m.shared.stats.Finished[0]
	second := first
	second.UserBookReads = m.shared.stats.Finished[1].UserBookReads
	m.shared.stats.Finished[1] = second
	s.rebuildFinished()
	before := stripANSI(m.detail.View(s.Selected(), tabStats))
	send(t, m, runeKey('j'))
	after := stripANSI(m.detail.View(s.Selected(), tabStats))
	if !strings.Contains(before, "20 Sep 2026") || !strings.Contains(after, "19 Sep 2026") {
		t.Fatal("reread selection kept stale completion date")
	}
	send(t, m, runeKey('G'))
	if s.finished.Index() != 15 {
		t.Fatal("G did not reach the last book")
	}
	send(t, m, runeKey('g'))
	if s.finished.Index() != 0 {
		t.Fatal("g did not reach the first book")
	}
	send(t, m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if s.finished.Index() == 0 {
		t.Fatal("half page navigation did not move")
	}
}

func TestPrivateNoteErrorsAndReviewSeparation(t *testing.T) {
	m := finishedModel(t, 80, 24)
	m.shared.privateNotes = map[int]string{1: "Never publish this"}
	send(t, m, runeKey('n'))
	n := m.topModal().(*journalModal)
	_, cmd := n.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if _, ok := cmd().(reqPrivateNote); !ok {
		t.Fatal("wrong destination")
	}
	done, _ := n.Update(opDoneMsg{op: opJournal, seq: n.token})
	if done || !n.submitting {
		t.Fatal("journal completion closed private editor")
	}
	done, _ = n.Update(opDoneMsg{op: opPrivateNote, seq: n.token, err: fmt.Errorf("disk full")})
	if done || n.submitting || n.text.Value() != "Never publish this" || n.err == "" {
		t.Fatal("failed save lost draft or error")
	}
	send(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	send(t, m, runeKey('v'))
	review := m.topModal().(*reviewModal)
	if strings.Contains(review.text.Value(), "Never publish this") {
		t.Fatal("private note leaked into Hardcover review")
	}
}

func TestGoldenPrivateNote(t *testing.T) {
	for _, size := range [][2]int{{40, 16}, {80, 24}, {120, 40}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := finishedModel(t, size[0], size[1])
			m.shared.privateNotes = map[int]string{1: "A book worth revisiting.\nKeep this thought private."}
			send(t, m, runeKey('n'))
			golden.RequireEqual(t, []byte(frameAt(m, layoutProfile)))
		})
	}
}
