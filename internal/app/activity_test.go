package app

import (
	"errors"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

func TestLogLocalJournalRecordsToday(t *testing.T) {
	pinUTC(t)
	a := newTestApp(t)

	a.logLocalJournal(journalEventProgress)

	now := time.Now().In(time.Local)
	days, err := a.store.GetJournalDays(now.AddDate(0, 0, -1), now)
	if err != nil {
		t.Fatalf("GetJournalDays: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("got %d days, want 1", len(days))
	}
	if got, want := days[0].Date.Format("2006-01-02"), now.Format("2006-01-02"); got != want {
		t.Fatalf("day = %s, want %s", got, want)
	}
	if days[0].Count != 1 {
		t.Fatalf("count = %d, want 1", days[0].Count)
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

func TestLenientPrivacyPrefersCacheThenFetchesThenFallsBack(t *testing.T) {
	// Offline, so the cached value can only come from state.
	a := newTestApp(t)
	if err := a.store.SetState(privacySettingKey, "3"); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	if got, err := a.privacy(t.Context(), false); got != 3 || err != nil {
		t.Fatalf("privacy = %d, %v; want cached 3", got, err)
	}

	hc := newFakeHardcover()
	hc.privacy = 2
	a = newTestAppWith(t, hc)
	if got, err := a.privacy(t.Context(), false); got != 2 || err != nil {
		t.Fatalf("privacy = %d, %v; want fetched 2", got, err)
	}
	if v, _ := a.store.GetState(privacySettingKey); v != "2" {
		t.Fatalf("privacy cached as %q, want 2", v)
	}

	hc = newFakeHardcover()
	hc.errs = map[string]error{"GetAccountPrivacySetting": errors.New("down")}
	a = newTestAppWith(t, hc)
	if got, err := a.privacy(t.Context(), false); got != privacyPublic || err != nil {
		t.Fatalf("privacy = %d, %v; want the public fallback", got, err)
	}
	if v, _ := a.store.GetState(privacySettingKey); v != "" {
		t.Fatalf("fallback cached as %q, want nothing", v)
	}
}

func TestStrictPrivacyFetchesFreshAndNeverFallsBack(t *testing.T) {
	hc := newFakeHardcover()
	hc.privacy = 2
	a := newTestAppWith(t, hc)
	if err := a.store.SetState(privacySettingKey, "3"); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	if got, err := a.privacy(t.Context(), true); got != 2 || err != nil {
		t.Fatalf("privacy = %d, %v; want fresh 2 over cached 3", got, err)
	}
	if v, _ := a.store.GetState(privacySettingKey); v != "2" {
		t.Fatalf("privacy cached as %q, want refreshed 2", v)
	}

	hc.errs = map[string]error{"GetAccountPrivacySetting": errors.New("down")}
	if got, err := a.privacy(t.Context(), true); err == nil {
		t.Fatalf("privacy = %d, want an error rather than the cached or public value", got)
	}
	hc.errs = nil
	hc.privacy = 0
	if got, err := a.privacy(t.Context(), true); err == nil {
		t.Fatalf("privacy = %d, want an error for an unset account setting", got)
	}

	offline := newTestApp(t)
	if err := offline.store.SetState(privacySettingKey, "3"); err != nil {
		t.Fatalf("SetState: %v", err)
	}
	if _, err := offline.privacy(t.Context(), true); !errors.Is(err, ErrOffline) {
		t.Fatalf("err = %v, want ErrOffline", err)
	}
}
