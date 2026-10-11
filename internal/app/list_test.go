package app

import (
	"context"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

func TestListBooksRechecksAnEmptyShelfButNotAFreshOne(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	ctx := context.Background()
	fresh := time.Now().UTC().Format(time.RFC3339)

	// A freshly synced empty shelf is still checked, so a book added on the
	// website shows at once.
	if err := a.store.SetState(syncStateKey(model.StatusPaused), fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ListBooks(ctx, model.StatusPaused, false); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, hc, "ListUserBooks", "ListUserBooks(4)")

	// A freshly synced shelf with books is served from the cache.
	upsertBookWithStatus(t, a, 1, 11, model.StatusCurrentlyReading, "Dune")
	if err := a.store.SetState(syncStateKey(model.StatusCurrentlyReading), fresh); err != nil {
		t.Fatal(err)
	}
	books, err := a.ListBooks(ctx, model.StatusCurrentlyReading, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Book.Title != "Dune" {
		t.Fatalf("ListBooks() = %+v, want the cached Dune", books)
	}
	assertCalls(t, hc, "ListUserBooks", "ListUserBooks(4)")
}
