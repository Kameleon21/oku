package tui

import (
	"bytes"
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// TestDashboardRunsInAProgram is the one test that runs the model through a
// real Bubble Tea program: Init starts the loads, the results land, the
// dashboard draws, and q quits. Everything else about the frames is pinned
// by the synchronous goldens, which have no goroutines to race.
func TestDashboardRunsInAProgram(t *testing.T) {
	d := demoData(fixedNow)
	// The demo library behind a fake backend: Init's loads run for real,
	// through the runner, and land as they would over the store.
	fb := newFakeBackend()
	fb.add(d.reading...)
	fb.add(d.oku...)
	fb.add(d.shelf[311])
	fb.stats, fb.sessions = d.stats, d.sessions
	fb.timer = &model.TimerState{BookID: 101, StartedAt: fixedNow.Add(-90 * time.Second)}
	fb.lastSync = fixedNow.Add(-2 * time.Minute)
	m := newModel(context.Background(), fb, DensityDefault, "test")
	m.shared.now = func() time.Time { return fixedNow }

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Reading (")) && bytes.Contains(b, []byte("The Communist Manifesto"))
	}, teatest.WithCheckInterval(10*time.Millisecond), teatest.WithDuration(5*time.Second))

	tm.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}
