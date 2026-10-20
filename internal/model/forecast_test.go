package model

import (
	"math"
	"testing"
	"time"
)

// The goal tests run on a 70-day window, 1 Jan to 11 Mar, so that 5 Feb is
// exactly halfway and five weeks remain.
var (
	goalStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	goalEnd   = time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	halfway   = time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)
)

func booksGoal(target int, progress float64) Goal {
	return Goal{Metric: "books", Target: target, Progress: progress, State: "active", StartDate: goalStart, EndDate: goalEnd}
}

func TestForecastGoalHalfway(t *testing.T) {
	p, ok := ForecastGoal(booksGoal(20, 8), halfway)
	if !ok {
		t.Fatal("a goal inside its window should forecast")
	}
	near := func(name string, got, want float64) {
		t.Helper()
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	near("Expected", p.Expected, 10)
	near("Ahead", p.Ahead, -2)
	near("PerWeek", p.PerWeek, 12.0/5)
	if p.Projected != 16 {
		t.Errorf("Projected = %d, want 16", p.Projected)
	}
	if p.DaysLeft != 35 {
		t.Errorf("DaysLeft = %d, want 35", p.DaysLeft)
	}
	if !p.End.Equal(goalEnd) {
		t.Errorf("End = %v, want %v", p.End, goalEnd)
	}
}

// TestForecastGoalWindow: the window is the goal's calendar days, both ends
// included, and nothing is forecast outside it.
func TestForecastGoalWindow(t *testing.T) {
	g := booksGoal(20, 3)
	cases := []struct {
		name string
		now  time.Time
		ok   bool
	}{
		{"the day before it starts", goalStart.Add(-time.Hour), false},
		{"its first morning", goalStart.Add(9 * time.Hour), true},
		{"the evening of its last day", goalEnd.Add(23 * time.Hour), true},
		{"the day after it ends", goalEnd.AddDate(0, 0, 1), false},
	}
	for _, c := range cases {
		if _, ok := ForecastGoal(g, c.now); ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
		}
	}

	last, _ := ForecastGoal(g, goalEnd.Add(23*time.Hour))
	if last.DaysLeft != 1 {
		t.Errorf("on the last day DaysLeft = %d, want 1 (today counts)", last.DaysLeft)
	}
	first, _ := ForecastGoal(g, goalStart.Add(9*time.Hour))
	if first.Projected != 0 {
		t.Errorf("Projected on the first day = %d, want 0: a few hours is no pace", first.Projected)
	}
}

func TestForecastGoalNeedsAWindowAndATarget(t *testing.T) {
	for name, g := range map[string]Goal{
		"no target":        {Metric: "books", StartDate: goalStart, EndDate: goalEnd},
		"no start date":    {Metric: "books", Target: 20, EndDate: goalEnd},
		"no end date":      {Metric: "books", Target: 20, StartDate: goalStart},
		"end before start": {Metric: "books", Target: 20, StartDate: goalEnd, EndDate: goalStart},
	} {
		if _, ok := ForecastGoal(g, halfway); ok {
			t.Errorf("%s: forecast a goal that cannot be measured", name)
		}
	}
}

// TestForecastGoalUsesTheReadersCalendar: Hardcover sends bare dates, which
// parse as UTC midnight. West of Greenwich that instant is the evening
// before, and the window must not lose its last day to it.
func TestForecastGoalUsesTheReadersCalendar(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	lastEvening := time.Date(2026, 3, 11, 21, 0, 0, 0, ny)
	p, ok := ForecastGoal(booksGoal(20, 19), lastEvening)
	if !ok {
		t.Fatal("the evening of the last day is inside the window")
	}
	if p.DaysLeft != 1 {
		t.Errorf("DaysLeft = %d, want 1", p.DaysLeft)
	}
}

func TestForecastBook(t *testing.T) {
	now := time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local)
	started := now.AddDate(0, 0, -4)
	ub := UserBook{
		StatusID:      StatusCurrentlyReading,
		Book:          Book{Pages: 210},
		UserBookReads: []UserBookRead{{ProgressPages: 120, StartedAt: &started}},
	}
	p, ok := ForecastBook(ub, now)
	if !ok {
		t.Fatal("an open read with a start date and pages should forecast")
	}
	if p.PagesPerDay != 30 || p.DaysLeft != 3 {
		t.Fatalf("pace = %v pages/day, %d days left; want 30 and 3", p.PagesPerDay, p.DaysLeft)
	}
	if want := time.Date(2026, 9, 8, 0, 0, 0, 0, time.Local); !p.Finish.Equal(want) {
		t.Fatalf("Finish = %v, want %v", p.Finish, want)
	}
}

func TestForecastReadDeclines(t *testing.T) {
	now := time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local)
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }
	cases := []struct {
		name        string
		started     time.Time
		page, pages int
		stalled     bool
	}{
		{"started today", now.Add(-3 * time.Hour), 40, 300, false},
		{"on the last page", day(10), 300, 300, false},
		{"stalled", day(400), 20, 3000, true},
		{"past two years", day(2), 2, 3000, false},
	}
	for _, c := range cases {
		p := ForecastRead(c.started, now, c.page, c.pages)
		if !p.Finish.IsZero() || p.DaysLeft != 0 {
			t.Errorf("%s: estimated %v (%d days)", c.name, p.Finish, p.DaysLeft)
		}
		if p.Stalled() != c.stalled {
			t.Errorf("%s: Stalled = %v, want %v", c.name, p.Stalled(), c.stalled)
		}
	}
}
