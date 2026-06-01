package app

import (
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

func upsertBookWithStatus(t *testing.T, a *App, userBookID, bookID int, status model.Status, title string) {
	t.Helper()
	ub := model.UserBook{
		ID:       userBookID,
		BookID:   bookID,
		StatusID: status,
		Book: model.Book{
			ID:      bookID,
			Title:   title,
			Authors: []string{"Author"},
			Pages:   100,
		},
		UpdatedAt: time.Now().UTC(),
	}
	if err := a.Store.UpsertUserBook(ub); err != nil {
		t.Fatalf("UpsertUserBook: %v", err)
	}
}

func TestGetUserBookForTitleExactMatch(t *testing.T) {
	a := newTestApp(t)
	upsertBookWithStatus(t, a, 1, 11, model.StatusCurrentlyReading, "Atomic Habits")
	upsertBookWithStatus(t, a, 2, 12, model.StatusWantToRead, "Deep Work")

	got, err := a.GetUserBookForTitle("atomic habits")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected exact match, got nil")
	}
	if got.Book.ID != 11 {
		t.Fatalf("matched book id = %d, want 11", got.Book.ID)
	}
}

func TestGetUserBookForTitleAmbiguousSubstringReturnsNil(t *testing.T) {
	a := newTestApp(t)
	upsertBookWithStatus(t, a, 1, 11, model.StatusCurrentlyReading, "Dune")
	upsertBookWithStatus(t, a, 2, 12, model.StatusWantToRead, "Dune Messiah")

	got, err := a.GetUserBookForTitle("du")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected ambiguous match to return nil, got %+v", got)
	}
}
