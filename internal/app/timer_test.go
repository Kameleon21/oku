package app

import (
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// pinUTC pins time.Local to UTC for the test: session bucketing is UTC-based
// while heatmap bucketing uses local day boundaries, and this keeps them
// aligned regardless of the machine's timezone.
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

func insertJournalDaysAgo(t *testing.T, a *App, daysAgo int, event string) {
	t.Helper()
	at := time.Now().UTC().AddDate(0, 0, -daysAgo).Truncate(24 * time.Hour).Add(12 * time.Hour)
	if err := a.Store.InsertLocalJournal(at, event); err != nil {
		t.Fatalf("InsertLocalJournal: %v", err)
	}
}

func TestGetHeatmapMarksJournalOnlyDays(t *testing.T) {
	pinUTC(t)
	a := newTestApp(t)

	insertSessionDaysAgo(t, a, 1, 30)
	insertJournalDaysAgo(t, a, 0, "progress_updated")

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
	if todayAct.Entries != 1 {
		t.Fatalf("today Entries = %d, want 1", todayAct.Entries)
	}
	if yesterdayAct == nil {
		t.Fatal("yesterday missing from heatmap")
	}
	if yesterdayAct.Minutes <= 0 {
		t.Fatalf("yesterday Minutes = %d, want > 0", yesterdayAct.Minutes)
	}
}
