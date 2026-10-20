package app

import (
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// TestGetReadingStatsForecasts: the stats carry the goal's pace and the open
// reads' finish estimates, from the cache alone.
func TestGetReadingStatsForecasts(t *testing.T) {
	a := newTestApp(t)
	upsertReadingBook(t, a, 7, 70, "Dune")
	started := time.Now().AddDate(0, 0, -10)
	if err := a.store.UpsertUserBookRead(model.UserBookRead{ID: 1, UserBookID: 7, ProgressPages: 100, StartedAt: &started}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := a.store.ReplaceGoals([]model.Goal{{
		ID: 1, Metric: "books", Target: 24, Progress: 3, State: "active",
		StartDate: now.AddDate(0, 0, -30), EndDate: now.AddDate(0, 0, 30),
	}}); err != nil {
		t.Fatal(err)
	}

	stats, err := a.GetReadingStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.GoalPace == nil || stats.GoalPace.Target != 24 || stats.GoalPace.Ahead >= 0 {
		t.Fatalf("GoalPace = %+v, want a 24-book goal running behind", stats.GoalPace)
	}
	if len(stats.Reading) != 1 || stats.Reading[0].Title != "Dune" || stats.Reading[0].Finish.IsZero() {
		t.Fatalf("Reading = %+v, want Dune with a finish date", stats.Reading)
	}
}

// TestGetReadingStatsNoGoalPaceOutsideItsWindow: a goal that has ended, the
// fallback pickBooksGoal keeps, has no pace to report.
func TestGetReadingStatsNoGoalPaceOutsideItsWindow(t *testing.T) {
	a := newTestApp(t)
	now := time.Now()
	if err := a.store.ReplaceGoals([]model.Goal{{
		ID: 1, Metric: "books", Target: 24, Progress: 3, State: "active",
		StartDate: now.AddDate(-1, 0, -10), EndDate: now.AddDate(0, 0, -10),
	}}); err != nil {
		t.Fatal(err)
	}
	stats, err := a.GetReadingStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Goal == nil {
		t.Fatal("the ended goal should still be the stats' goal")
	}
	if stats.GoalPace != nil {
		t.Fatalf("GoalPace = %+v, want nil for an ended goal", stats.GoalPace)
	}
}
