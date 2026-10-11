package app

import (
	"context"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// seedAccount caches a reading book for user 1 with a fresh stamp and a
// queue, the state a signed-in account leaves behind.
func seedAccount(t *testing.T, a *App) {
	t.Helper()
	upsertBookWithStatus(t, a, 1, 11, model.StatusCurrentlyReading, "Dune")
	for key, value := range map[string]string{
		userIDKey: "1",
		syncStateKey(model.StatusCurrentlyReading): time.Now().UTC().Format(time.RFC3339),
		queueStateKey:     "[11]",
		privacySettingKey: "3",
	} {
		if err := a.store.SetState(key, value); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnotherAccountClearsTheCachedLibrary(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	seedAccount(t, a)

	hc.userID = 2
	if _, _, err := a.ValidateToken(context.Background()); err != nil {
		t.Fatal(err)
	}

	if books, _ := a.CachedBooks(model.StatusCurrentlyReading); len(books) != 0 {
		t.Fatalf("cached reading = %+v, want the previous account's books gone", books)
	}
	for _, key := range []string{syncStateKey(model.StatusCurrentlyReading), queueStateKey, privacySettingKey} {
		if v, _ := a.store.GetState(key); v != "" {
			t.Fatalf("state %s = %q, want it cleared", key, v)
		}
	}
	if v, _ := a.store.GetState(userIDKey); v != "2" {
		t.Fatalf("user_id = %q, want the new account", v)
	}

	if _, err := a.ListBooks(context.Background(), model.StatusCurrentlyReading, false); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, hc, "ListUserBooks", "ListUserBooks(2)")
}

func TestSameAccountKeepsTheCachedLibrary(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	seedAccount(t, a)

	if _, _, err := a.ValidateToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if books, _ := a.CachedBooks(model.StatusCurrentlyReading); len(books) != 1 {
		t.Fatalf("cached reading = %+v, want Dune kept", books)
	}
	if _, err := a.ListBooks(context.Background(), model.StatusCurrentlyReading, false); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, hc, "ListUserBooks")
}

func TestSyncAllNoticesAnotherAccount(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	seedAccount(t, a)

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
