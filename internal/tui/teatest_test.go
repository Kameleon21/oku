package tui

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

// TestDashboardRunsInAProgram is the one test that runs the model through a
// real Bubble Tea program: Init starts the loads, the results land, the
// dashboard draws, and q quits. Everything else about the frames is pinned
// by the synchronous goldens, which have no goroutines to race.
func TestDashboardRunsInAProgram(t *testing.T) {
	d := demoData(fixedNow)
	// No app: the loads report their own errors, and the messages below are
	// what a real load would have delivered.
	m := New(context.Background(), nil, DensityDefault, "test")
	m.shared.now = func() time.Time { return fixedNow }

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	tm.Send(libraryLoadedMsg{reading: d.reading, oku: d.oku})
	tm.Send(localDataLoadedMsg{
		readingStats:   d.stats,
		recentSessions: d.sessions,
		shelf:          d.shelf,
		timerState:     &model.TimerState{BookID: 101, StartedAt: fixedNow.Add(-90 * time.Second)},
		timerBook:      &d.reading[0].Book,
		lastSyncAt:     fixedNow.Add(-2 * time.Minute),
	})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Reading (")) && bytes.Contains(b, []byte("The Communist Manifesto"))
	}, teatest.WithCheckInterval(10*time.Millisecond), teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}
