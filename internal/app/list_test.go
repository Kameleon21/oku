package app

import (
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

	if err := a.Store.SetState(
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
