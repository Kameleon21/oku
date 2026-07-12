package app

import (
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// pinUTC pins time.Local to UTC for the test: session bucketing is UTC-based
// while GetStreak uses local day boundaries, and this keeps them aligned
// regardless of the machine's timezone.
func pinUTC(t *testing.T) {
	t.Helper()
	orig := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = orig })
}

func insertSessionDaysAgo(t *testing.T, a *App, daysAgo, minutes int) {
	t.Helper()
	start := time.Now().UTC().AddDate(0, 0, -daysAgo).Truncate(24 * time.Hour).Add(12 * time.Hour)
	end := start.Add(time.Duration(minutes) * time.Minute)
	if _, err := a.Store.InsertSession(model.ReadingSession{
		BookID:    1,
		StartedAt: start,
		EndedAt:   &end,
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
}

func TestGetStreakKeepsLongestWhenCurrentBroken(t *testing.T) {
	pinUTC(t)
	a := newTestApp(t)

	// A 3-day run that ended days ago: nothing today or yesterday.
	for _, daysAgo := range []int{5, 4, 3} {
		insertSessionDaysAgo(t, a, daysAgo, 30)
	}

	streak, err := a.GetStreak()
	if err != nil {
		t.Fatal(err)
	}
	if streak.Current != 0 {
		t.Fatalf("Current = %d, want 0 (streak is broken)", streak.Current)
	}
	if streak.Longest != 3 {
		t.Fatalf("Longest = %d, want 3 (record must survive a broken streak)", streak.Longest)
	}
	if streak.Total != 3 {
		t.Fatalf("Total = %d, want 3", streak.Total)
	}
	if streak.ReadToday {
		t.Fatal("ReadToday = true, want false")
	}
}

func TestGetStreakCurrentRun(t *testing.T) {
	pinUTC(t)
	a := newTestApp(t)

	// Read the last two days including today.
	insertSessionDaysAgo(t, a, 1, 30)
	insertSessionDaysAgo(t, a, 0, 30)

	streak, err := a.GetStreak()
	if err != nil {
		t.Fatal(err)
	}
	if streak.Current != 2 {
		t.Fatalf("Current = %d, want 2", streak.Current)
	}
	if streak.Longest != 2 {
		t.Fatalf("Longest = %d, want 2", streak.Longest)
	}
}
