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

// TestPickBooksGoalOnItsLastEvening: west of Greenwich, the last evening of
// a goal is already the next day in UTC; the goal is still the current one.
func TestPickBooksGoalOnItsLastEvening(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	// Next year's goal is listed first, so only the window check keeps it
	// from winning, and a pages goal is there to be skipped.
	goals := []model.Goal{
		{ID: 1, Metric: "pages", Target: 9000, State: "active", StartDate: day(2026, 1, 1), EndDate: day(2026, 12, 31)},
		{ID: 2027, Metric: "books", Target: 30, State: "active", StartDate: day(2027, 1, 1), EndDate: day(2027, 12, 31)},
		{ID: 2026, Metric: "books", Target: 24, State: "active", StartDate: day(2026, 1, 1), EndDate: day(2026, 12, 31)},
	}
	lastEvening := time.Date(2026, 12, 31, 20, 0, 0, 0, ny)
	if g := pickBooksGoal(goals, lastEvening); g == nil || g.ID != 2026 {
		t.Fatalf("pickBooksGoal = %+v, want the 2026 goal", g)
	}
	if g := pickBooksGoal(goals, lastEvening.Add(5*time.Hour)); g == nil || g.ID != 2027 {
		t.Fatalf("after midnight pickBooksGoal = %+v, want the 2027 goal", g)
	}
}

// TestPickBooksGoalFallbacks: with no goal covering today, an upcoming one
// beats one that has ended, and an ended one beats none.
func TestPickBooksGoalFallbacks(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	ended := model.Goal{ID: 1, Metric: "books", State: "active", StartDate: day(2025, 1, 1), EndDate: day(2025, 12, 31)}
	upcoming := model.Goal{ID: 2, Metric: "books", State: "active", StartDate: day(2027, 1, 1), EndDate: day(2027, 12, 31)}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.Local)

	if g := pickBooksGoal([]model.Goal{ended, upcoming}, now); g == nil || g.ID != 2 {
		t.Fatalf("pickBooksGoal = %+v, want the upcoming goal", g)
	}
	if g := pickBooksGoal([]model.Goal{ended}, now); g == nil || g.ID != 1 {
		t.Fatalf("pickBooksGoal = %+v, want the ended goal as a last resort", g)
	}
	if g := pickBooksGoal(nil, now); g != nil {
		t.Fatalf("pickBooksGoal(nil) = %+v, want nil", g)
	}
}
