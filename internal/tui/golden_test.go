package tui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/exp/golden"
)

// The golden frames are synchronous snapshots: a model is built from the
// demo fixtures on a fixed clock, fed a window size and whatever keys the
// case needs, and its View is compared byte for byte. No program runs, so
// there is no goroutine, no tick and no spinner frame to race.
//
// Regenerate them all with:
//
//	go test ./internal/tui -update
//
// The layout cases are written at the NoTTY profile, so a diff shows the
// frame and not a wall of escape sequences; two Reading cases are written in
// TrueColor, one for a light terminal and one for a dark one, to keep the
// palette itself covered. lipgloss v2 has no global profile, so the frame is
// downsampled by the test rather than by a setting (see frameAt).

// goldenOpt arranges a golden model before it is rendered.
type goldenOpt func(*Model)

// newGoldenModel builds a loaded dashboard of the given size on tab tb, with
// the demo library, stats, sessions and shelf behind it.
func newGoldenModel(t *testing.T, w, h int, tb tab, opts ...goldenOpt) *Model {
	t.Helper()
	d := demoData(fixedNow)

	m := newTestModel()
	m.shared.now = func() time.Time { return fixedNow }
	m.shared.loaded, m.shared.localLoaded = true, true
	m.shared.reading, m.shared.oku = d.reading, d.oku
	m.shared.shelf = d.shelf
	m.shared.stats, m.shared.weekly = d.stats, d.stats.Weekly
	m.shared.sessions = d.sessions
	m.shared.recentSearches = d.recentSearches
	// A fixed age, so "synced 2m ago" does not move with the wall clock.
	m.shared.lastSyncAt = fixedNow.Add(-2 * time.Minute)
	m.broadcast(dataChangedMsg{dataLibrary})

	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m.setTab(tb)
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// withDetailFocus presses Enter, which is the only way a reader reaches the
// detail pane: a golden taken from a focus no key can produce would prove
// nothing about the dashboard.
func withDetailFocus(m *Model) {
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
}

// withResults fills the Search tab with the demo results and puts the cursor
// in them.
func withResults(m *Model) {
	s := searchOf(m)
	s.results = demoData(fixedNow).results
	s.lastQuery, s.lastMode = "dune", "book"
	s.input.SetValue("dune")
	s.rebuildResults()
	s.focusResults()
}

// withBackground is the terminal answering the background query, which is
// what a light palette hangs off now that lipgloss has no adaptive colour.
func withBackground(dark bool) goldenOpt {
	return func(m *Model) { m.Update(backgroundMsg(dark)) }
}

// withRunningTimer puts a timer in the header.
func withRunningTimer(m *Model) {
	m.shared.timer = &model.TimerState{BookID: 101, StartedAt: fixedNow.Add(-12*time.Minute - 41*time.Second)}
	m.shared.timerBook = &m.shared.reading[0].Book
}

// TestGoldenTabs is the frame of every tab at both sizes, with the keyboard
// on the content pane and on the detail pane.
func TestGoldenTabs(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		for _, tb := range allTabs {
			for _, f := range []struct {
				name string
				opts []goldenOpt
			}{
				{"content", nil},
				{"detail", []goldenOpt{withDetailFocus}},
			} {
				name := fmt.Sprintf("%s_%dx%d_%s", tb.name(), size[0], size[1], f.name)
				t.Run(name, func(t *testing.T) {
					opts := f.opts
					if tb == tabSearch {
						opts = append([]goldenOpt{withResults}, opts...)
					}
					m := newGoldenModel(t, size[0], size[1], tb, opts...)
					golden.RequireEqual(t, []byte(frameAt(m, layoutProfile)))
				})
			}
		}
	}
}

// TestGoldenSearchStates covers the two states of the Search tab, plus the
// tab as it is first reached: nothing searched for yet.
func TestGoldenSearchStates(t *testing.T) {
	cases := map[string]goldenOpt{
		"input":   func(m *Model) { searchOf(m).focusInput() },
		"results": func(m *Model) { searchOf(m).focusResults() },
		"empty":   func(*Model) {},
	}
	for name, opt := range cases {
		t.Run(name, func(t *testing.T) {
			opts := []goldenOpt{opt}
			if name != "empty" {
				opts = append([]goldenOpt{withResults}, opts...)
			}
			m := newGoldenModel(t, 120, 40, tabSearch, opts...)
			golden.RequireEqual(t, []byte(frameAt(m, layoutProfile)))
		})
	}
}

// TestGoldenModals is each overlay over the Reading tab.
func TestGoldenModals(t *testing.T) {
	cases := map[string]goldenOpt{
		"help":    func(m *Model) { m.openHelp() },
		"page":    func(m *Model) { m.push(newPageModal(m.shared, m.st, m.shared.reading[0])) },
		"review":  func(m *Model) { m.push(newReviewModal(m.shared, m.st, m.shared.reading[0])) },
		"confirm": func(m *Model) { m.push(newConfirmModal("Mark 'The Communist Manifesto' as Ignored?", nil)) },
		"timer":   func(m *Model) { m.push(newTimerPickerModal(m.shared, 1)) },
	}
	for name, opt := range cases {
		t.Run(name, func(t *testing.T) {
			m := newGoldenModel(t, 120, 40, tabReading, opt)
			golden.RequireEqual(t, []byte(frameAt(m, layoutProfile)))
		})
	}
}

// TestGoldenHeaderStates covers the header's own decisions: the running
// timer, and what it drops as the terminal narrows.
func TestGoldenHeaderStates(t *testing.T) {
	for _, w := range []int{140, 120, 100, 80, 60} {
		t.Run(fmt.Sprintf("timer_%d", w), func(t *testing.T) {
			m := newGoldenModel(t, w, 24, tabReading, withRunningTimer)
			golden.RequireEqual(t, []byte(atProfile(m.header(m.lay), layoutProfile)))
		})
	}
	t.Run("never_synced", func(t *testing.T) {
		m := newGoldenModel(t, 120, 24, tabReading, func(m *Model) { m.shared.lastSyncAt = time.Time{} })
		golden.RequireEqual(t, []byte(atProfile(m.header(m.lay), layoutProfile)))
	})
	t.Run("syncing", func(t *testing.T) {
		m := newGoldenModel(t, 120, 24, tabReading, func(m *Model) { m.syncing = true })
		golden.RequireEqual(t, []byte(atProfile(m.header(m.lay), layoutProfile)))
	})
}

// TestGoldenToast pins the footer with a message and an undo on offer.
func TestGoldenToast(t *testing.T) {
	m := newGoldenModel(t, 120, 40, tabReading, func(m *Model) {
		m.showUndoToast("Page 70", undoAction{op: opProgress, bookID: 101, title: "The Communist Manifesto",
			toPage: 60, fromPage: 70})
	})
	golden.RequireEqual(t, []byte(frameAt(m, layoutProfile)))
}

// TestGoldenColour renders the same frame in colour, on a dark terminal and
// on a light one, so a palette change shows up as a diff.
func TestGoldenColour(t *testing.T) {
	for name, dark := range map[string]bool{"dark": true, "light": false} {
		t.Run(name, func(t *testing.T) {
			m := newGoldenModel(t, 120, 40, tabReading, withBackground(dark))
			golden.RequireEqual(t, []byte(frameAt(m, colorprofile.TrueColor)))
		})
	}

	// One frame per named palette, so a change to any of them shows up as a
	// diff and a reviewer can read the hexes out of the frame. The palette is
	// chosen the way the config key chooses it — before the model is built,
	// which is when lipgloss v2 resolves the colours — and the terminal is
	// never asked, so no background message is fed in.
	for _, nt := range NamedThemes() {
		t.Run(nt.Name, func(t *testing.T) {
			if err := ApplyThemeSetting(nt.Name); err != nil {
				t.Fatalf("ApplyThemeSetting(%s) error = %v", nt.Name, err)
			}
			t.Cleanup(func() { _ = ApplyThemeSetting("auto") })

			m := newGoldenModel(t, 120, 40, tabReading)
			golden.RequireEqual(t, []byte(frameAt(m, colorprofile.TrueColor)))
		})
	}
}
