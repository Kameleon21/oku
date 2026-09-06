package app

import (
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

func TestLastSyncAtIsTheNewestStatusStamp(t *testing.T) {
	a := newTestApp(t)

	if got := a.LastSyncAt(); !got.IsZero() {
		t.Fatalf("LastSyncAt() = %v before any sync, want the zero time", got)
	}

	older := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	newer := older.Add(2 * time.Hour)
	for status, at := range map[model.Status]time.Time{
		model.StatusCurrentlyReading: older,
		model.StatusWantToRead:       newer,
	} {
		if err := a.Store.SetState(syncStateKey(status), at.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}
	// A stamp that does not parse is skipped, not fatal.
	if err := a.Store.SetState(syncStateKey(model.StatusRead), "not a time"); err != nil {
		t.Fatal(err)
	}

	if got := a.LastSyncAt(); !got.Equal(newer) {
		t.Fatalf("LastSyncAt() = %v, want the newest stamp %v", got, newer)
	}
}
