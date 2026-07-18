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

func insertActivityDaysAgo(t *testing.T, a *App, daysAgo int, event string) {
	t.Helper()
	at := time.Now().UTC().AddDate(0, 0, -daysAgo).Truncate(24 * time.Hour).Add(12 * time.Hour)
	if err := a.Store.InsertActivity(1, event, at); err != nil {
		t.Fatalf("InsertActivity: %v", err)
	}
}

func TestGetStreakUnionOfSessionsAndActivity(t *testing.T) {
	pinUTC(t)
	a := newTestApp(t)

	// Timer session two days ago, progress updates yesterday and today.
	insertSessionDaysAgo(t, a, 2, 30)
	insertActivityDaysAgo(t, a, 1, "progress")
	insertActivityDaysAgo(t, a, 0, "finished")

	streak, err := a.GetStreak()
	if err != nil {
		t.Fatal(err)
	}
	if streak.Current != 3 {
		t.Fatalf("Current = %d, want 3", streak.Current)
	}
	if streak.Total != 3 {
		t.Fatalf("Total = %d, want 3", streak.Total)
	}
	if !streak.ReadToday {
		t.Fatal("ReadToday = false, want true")
	}
}

func TestGetStreakActivityOnlyNoSessions(t *testing.T) {
	pinUTC(t)
	a := newTestApp(t)

	// No timer sessions at all — only progress updates.
	insertActivityDaysAgo(t, a, 1, "progress")
	insertActivityDaysAgo(t, a, 0, "progress")

	streak, err := a.GetStreak()
	if err != nil {
		t.Fatal(err)
	}
	if streak.Current != 2 {
		t.Fatalf("Current = %d, want 2", streak.Current)
	}
	if !streak.ReadToday {
		t.Fatal("ReadToday = false, want true")
	}
}

func TestGetStreakSameDaySessionAndActivityCountsOnce(t *testing.T) {
	pinUTC(t)
	a := newTestApp(t)

	insertSessionDaysAgo(t, a, 0, 30)
	insertActivityDaysAgo(t, a, 0, "progress")

	streak, err := a.GetStreak()
	if err != nil {
		t.Fatal(err)
	}
	if streak.Total != 1 {
		t.Fatalf("Total = %d, want 1", streak.Total)
	}
	if streak.Current != 1 {
		t.Fatalf("Current = %d, want 1", streak.Current)
	}
}

func TestGetHeatmapMarksUpdateOnlyDays(t *testing.T) {
	pinUTC(t)
	a := newTestApp(t)

	insertSessionDaysAgo(t, a, 1, 30)
	insertActivityDaysAgo(t, a, 0, "progress")

	acts, err := a.GetHeatmap(4)
	if err != nil {
		t.Fatal(err)
	}

	today := time.Now().In(time.Local).Format("2006-01-02")
	yesterday := time.Now().In(time.Local).AddDate(0, 0, -1).Format("2006-01-02")
	var todayAct, yesterdayAct *model.DayActivity
	for i := range acts {
		switch acts[i].Date.Format("2006-01-02") {
		case today:
			todayAct = &acts[i]
		case yesterday:
			yesterdayAct = &acts[i]
		}
	}

	if todayAct == nil {
		t.Fatal("today missing from heatmap")
	}
	if todayAct.Minutes != 0 || !todayAct.HasActivity {
		t.Fatalf("today = {Minutes: %d, HasActivity: %v}, want {0, true}", todayAct.Minutes, todayAct.HasActivity)
	}
	if yesterdayAct == nil {
		t.Fatal("yesterday missing from heatmap")
	}
	if yesterdayAct.Minutes <= 0 {
		t.Fatalf("yesterday Minutes = %d, want > 0", yesterdayAct.Minutes)
	}
}
