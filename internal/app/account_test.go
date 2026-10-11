package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// seedAccount caches what a signed-in account leaves behind: a reading and
// a read book with a fresh stamp, a queue, a privacy setting, a synced and a
// local journal entry, a goal, and the recorded user ID. Device-local data
// (a private note, a timer session) is there too.
func seedAccount(t *testing.T, a *App, userID string) {
	t.Helper()
	upsertBookWithStatus(t, a, 1, 11, model.StatusCurrentlyReading, "Dune")
	upsertBookWithStatus(t, a, 2, 12, model.StatusRead, "Emma")
	now := time.Now().UTC()
	for key, value := range map[string]string{
		userIDKey: userID,
		syncStateKey(model.StatusCurrentlyReading): now.Format(time.RFC3339),
		queueStateKey:     "[11]",
		privacySettingKey: "3",
	} {
		if err := a.store.SetState(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.store.ReplaceReadingJournals([]model.JournalEntry{{ID: 5, ActionAt: now, Event: "progress_updated"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.store.InsertLocalJournal(now, "progress_updated"); err != nil {
		t.Fatal(err)
	}
	if err := a.store.ReplaceGoals([]model.Goal{{ID: 1, Metric: "book", Target: 12}}); err != nil {
		t.Fatal(err)
	}
	if err := a.SavePrivateNote(11, "Mine."); err != nil {
		t.Fatal(err)
	}
}

func journalDays(t *testing.T, a *App) int {
	t.Helper()
	now := time.Now()
	days, err := a.store.GetJournalDays(now.AddDate(0, 0, -2), now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	return len(days)
}

func TestAnotherAccountClearsTheCachedLibrary(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	seedAccount(t, a, "1")

	if cleared, err := a.RememberUser(2); err != nil || !cleared {
		t.Fatalf("RememberUser() = %v, %v; want a clear", cleared, err)
	}

	for _, status := range []model.Status{model.StatusCurrentlyReading, model.StatusRead} {
		if books, _ := a.CachedBooks(status); len(books) != 0 {
			t.Fatalf("cached %s = %+v, want the previous account's books gone", status, books)
		}
	}
	for _, key := range []string{syncStateKey(model.StatusCurrentlyReading), queueStateKey, privacySettingKey} {
		if v, _ := a.store.GetState(key); v != "" {
			t.Fatalf("state %s = %q, want it cleared", key, v)
		}
	}
	if n := journalDays(t, a); n != 0 {
		t.Fatalf("journal days = %d, want the synced and local entries gone", n)
	}
	if goals, _ := a.store.ListGoals(); len(goals) != 0 {
		t.Fatalf("goals = %+v, want them gone", goals)
	}
	if v, _ := a.store.GetState(userIDKey); v != "2" {
		t.Fatalf("user_id = %q, want the new account", v)
	}
	if notes, _ := a.ListPrivateNotes(); notes[11] != "Mine." {
		t.Fatalf("private notes = %v, want the device-local note kept", notes)
	}

	if _, err := a.ListBooks(context.Background(), model.StatusCurrentlyReading, false); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, hc, "ListUserBooks", "ListUserBooks(2)")
}

func TestRememberUserKeepsTheCacheWithoutAPreviousAccount(t *testing.T) {
	for name, stored := range map[string]string{
		"same account": "2",
		"first run":    "",
		"zero":         "0",
		"garbage":      "not-a-number",
	} {
		t.Run(name, func(t *testing.T) {
			hc := newFakeHardcover()
			a := newTestAppWith(t, hc)
			seedAccount(t, a, stored)

			if cleared, err := a.RememberUser(2); err != nil || cleared {
				t.Fatalf("RememberUser() = %v, %v; want no clear", cleared, err)
			}
			if books, _ := a.CachedBooks(model.StatusCurrentlyReading); len(books) != 1 {
				t.Fatalf("cached reading = %+v, want Dune kept", books)
			}
			if v, _ := a.store.GetState(queueStateKey); v != "[11]" {
				t.Fatalf("queue order = %q, want it kept", v)
			}
			if v, _ := a.store.GetState(userIDKey); v != "2" {
				t.Fatalf("user_id = %q, want 2", v)
			}
		})
	}
}

func TestSyncAllNoticesAnotherAccount(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	seedAccount(t, a, "1")

	hc.userID = 2
	if err := a.SyncAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.store.GetState(queueStateKey); v != "" {
		t.Fatalf("queue order = %q, want the previous account's cleared", v)
	}
	if v, _ := a.store.GetState(userIDKey); v != "2" {
		t.Fatalf("user_id = %q, want the new account", v)
	}
}

func TestSyncAllGoesOnWhenTheAccountCheckFails(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	seedAccount(t, a, "1")
	hc.errs = map[string]error{"GetMe": errors.New("timeout")}

	if err := a.SyncAll(context.Background()); err != nil {
		t.Fatalf("SyncAll() = %v, want the sync to go on without the account check", err)
	}
	if got := hc.called("ListUserBooks"); len(got) != len(model.AllStatuses) {
		t.Fatalf("ListUserBooks calls = %v, want every status synced", got)
	}
	if v, _ := a.store.GetState(userIDKey); v != "1" {
		t.Fatalf("user_id = %q, want the recorded account kept", v)
	}
}
