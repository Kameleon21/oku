package app

import (
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

func TestLogActivityRecordsToday(t *testing.T) {
	pinUTC(t)
	a := newTestApp(t)

	a.logActivity(1, activityProgress)

	now := time.Now().In(time.Local)
	days, err := a.Store.GetActivityDays(now.AddDate(0, 0, -1), now)
	if err != nil {
		t.Fatalf("GetActivityDays: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("got %d days, want 1", len(days))
	}
	if got, want := days[0].Format("2006-01-02"), now.Format("2006-01-02"); got != want {
		t.Fatalf("day = %s, want %s", got, want)
	}
}

func TestStatusCountsAsActivity(t *testing.T) {
	cases := []struct {
		status model.Status
		want   bool
	}{
		{model.StatusWantToRead, false},
		{model.StatusCurrentlyReading, false},
		{model.StatusRead, true},
		{model.StatusPaused, false},
		{model.StatusDidNotFinish, false},
		{model.StatusIgnored, false},
	}
	for _, c := range cases {
		if got := statusCountsAsActivity(c.status); got != c.want {
			t.Errorf("statusCountsAsActivity(%v) = %v, want %v", c.status, got, c.want)
		}
	}
}
