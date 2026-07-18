package store

import (
	"testing"
	"time"
)

func TestInsertActivityAndGetActivityDays(t *testing.T) {
	s := testStore(t)

	// Noon UTC keeps the local day stable across timezones.
	dayAt := func(daysAgo int) time.Time {
		return time.Now().UTC().AddDate(0, 0, -daysAgo).Truncate(24 * time.Hour).Add(12 * time.Hour)
	}

	// Three events on the same day must collapse to one; one event is
	// outside the queried range.
	inserts := []struct {
		daysAgo int
		event   string
	}{
		{0, "progress"},
		{0, "progress"},
		{0, "finished"},
		{1, "progress"},
		{3, "progress"},
		{10, "progress"},
	}
	for _, in := range inserts {
		if err := s.InsertActivity(1, in.event, dayAt(in.daysAgo)); err != nil {
			t.Fatalf("InsertActivity: %v", err)
		}
	}

	from := dayAt(5).In(time.Local)
	to := dayAt(0).In(time.Local)
	days, err := s.GetActivityDays(from, to)
	if err != nil {
		t.Fatalf("GetActivityDays: %v", err)
	}

	want := []string{
		dayAt(3).In(time.Local).Format("2006-01-02"),
		dayAt(1).In(time.Local).Format("2006-01-02"),
		dayAt(0).In(time.Local).Format("2006-01-02"),
	}
	if len(days) != len(want) {
		t.Fatalf("got %d days, want %d (%v)", len(days), len(want), days)
	}
	for i, d := range days {
		if got := d.Format("2006-01-02"); got != want[i] {
			t.Errorf("day[%d] = %s, want %s", i, got, want[i])
		}
	}
}
