package app

import (
	"context"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

func TestListCachedBooksReportsRefreshState(t *testing.T) {
	a := newTestApp(t)
	upsertBookWithStatus(t, a, 1, 11, model.StatusCurrentlyReading, "Dune")

	books, needsRefresh, err := a.ListCachedBooks(model.StatusCurrentlyReading)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Book.Title != "Dune" {
		t.Fatalf("ListCachedBooks() = %#v, want Dune", books)
	}
	if !needsRefresh {
		t.Fatal("cache without sync state should need refresh")
	}

	if err := a.store.SetState(
		syncStateKey(model.StatusCurrentlyReading),
		time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}
	_, needsRefresh, err = a.ListCachedBooks(model.StatusCurrentlyReading)
	if err != nil {
		t.Fatal(err)
	}
	if needsRefresh {
		t.Fatal("recently synced cache should not need refresh")
	}
}

func TestEmptyShelfFollowsTheSyncStamp(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	ctx := context.Background()

	// Never synced: an empty shelf is unknown, so it needs a fetch.
	if _, stale, err := a.ListCachedBooks(model.StatusPaused); err != nil || !stale {
		t.Fatalf("ListCachedBooks() before any sync: stale = %v, err = %v; want stale", stale, err)
	}
	if _, err := a.ListBooks(ctx, model.StatusPaused, false); err != nil {
		t.Fatal(err)
	}
	if got := hc.called("ListUserBooks"); len(got) != 1 {
		t.Fatalf("ListUserBooks calls = %v, want one for the unsynced shelf", got)
	}

	// Synced and still empty: up to date, so no further request.
	if _, stale, err := a.ListCachedBooks(model.StatusPaused); err != nil || stale {
		t.Fatalf("ListCachedBooks() after sync: stale = %v, err = %v; want fresh", stale, err)
	}
	if _, err := a.ListBooks(ctx, model.StatusPaused, false); err != nil {
		t.Fatal(err)
	}
	if got := hc.called("ListUserBooks"); len(got) != 1 {
		t.Fatalf("ListUserBooks calls = %v, want no fetch for a freshly synced empty shelf", got)
	}

	// Once the stamp is old the shelf is fetched again.
	old := time.Now().Add(-cacheStaleAfter - time.Minute).UTC().Format(time.RFC3339)
	if err := a.store.SetState(syncStateKey(model.StatusPaused), old); err != nil {
		t.Fatal(err)
	}
	if _, stale, err := a.ListCachedBooks(model.StatusPaused); err != nil || !stale {
		t.Fatalf("ListCachedBooks() with an old stamp: stale = %v, err = %v; want stale", stale, err)
	}
	if _, err := a.ListBooks(ctx, model.StatusPaused, false); err != nil {
		t.Fatal(err)
	}
	if got := hc.called("ListUserBooks"); len(got) != 2 {
		t.Fatalf("ListUserBooks calls = %v, want a second fetch for the stale shelf", got)
	}
}
