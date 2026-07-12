package app

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/store"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return &App{Store: s}
}

func upsertReadingBook(t *testing.T, a *App, userBookID, bookID int, title string) {
	t.Helper()
	ub := model.UserBook{
		ID:       userBookID,
		BookID:   bookID,
		StatusID: model.StatusCurrentlyReading,
		Book: model.Book{
			ID:      bookID,
			Title:   title,
			Authors: []string{"Author"},
			Pages:   300,
		},
		UpdatedAt: time.Now(),
	}
	if err := a.Store.UpsertUserBook(ub); err != nil {
		t.Fatalf("UpsertUserBook: %v", err)
	}
}

func TestGetActiveBookIDsMigratesLegacyState(t *testing.T) {
	a := newTestApp(t)
	if err := a.Store.SetState(activeBookIDKey, "42"); err != nil {
		t.Fatal(err)
	}

	ids, err := a.GetActiveBookIDs()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []int{42}) {
		t.Fatalf("GetActiveBookIDs() = %v, want [42]", ids)
	}

	raw, err := a.Store.GetState(activeBookIDsKey)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "[42]" {
		t.Fatalf("active_book_ids = %q, want [42]", raw)
	}

	legacy, err := a.Store.GetState(activeBookIDKey)
	if err != nil {
		t.Fatal(err)
	}
	if legacy != "" {
		t.Fatalf("legacy active_book_id = %q, want deleted", legacy)
	}
}

func TestRemovedBookDoesNotResurrectFromLegacyState(t *testing.T) {
	a := newTestApp(t)
	if err := a.Store.SetState(activeBookIDKey, "42"); err != nil {
		t.Fatal(err)
	}

	// Trigger migration, then remove the book.
	if _, err := a.GetActiveBookIDs(); err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveActiveBook(42); err != nil {
		t.Fatal(err)
	}

	ids, err := a.GetActiveBookIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("GetActiveBookIDs() = %v, want empty after removal", ids)
	}
}

func TestActiveBooksSupportMultipleWithoutPrimary(t *testing.T) {
	a := newTestApp(t)
	upsertReadingBook(t, a, 100, 1, "Book One")
	upsertReadingBook(t, a, 101, 2, "Book Two")

	if err := a.SetActiveBook(1); err != nil {
		t.Fatal(err)
	}
	if err := a.AddActiveBook(2); err != nil {
		t.Fatal(err)
	}

	ids, err := a.GetActiveBookIDs()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []int{1, 2}) {
		t.Fatalf("GetActiveBookIDs() = %v, want [1 2]", ids)
	}

	if _, err := a.GetActiveBookID(); err == nil {
		t.Fatal("GetActiveBookID() expected error for multiple active books")
	}
}

func TestGetActiveBookIDFallsBackToSingleActiveEntry(t *testing.T) {
	a := newTestApp(t)
	upsertReadingBook(t, a, 100, 7, "Single Active")

	if err := a.AddActiveBook(7); err != nil {
		t.Fatal(err)
	}

	id, err := a.GetActiveBookID()
	if err != nil {
		t.Fatal(err)
	}
	if id != 7 {
		t.Fatalf("GetActiveBookID() = %d, want 7", id)
	}
}

func TestRemoveActiveBookLeavesSingleDefault(t *testing.T) {
	a := newTestApp(t)
	upsertReadingBook(t, a, 100, 1, "Book One")
	upsertReadingBook(t, a, 101, 2, "Book Two")

	if err := a.SetActiveBook(1); err != nil {
		t.Fatal(err)
	}
	if err := a.AddActiveBook(2); err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveActiveBook(1); err != nil {
		t.Fatal(err)
	}

	ids, err := a.GetActiveBookIDs()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []int{2}) {
		t.Fatalf("GetActiveBookIDs() = %v, want [2]", ids)
	}

	id, err := a.GetActiveBookID()
	if err != nil {
		t.Fatal(err)
	}
	if id != 2 {
		t.Fatalf("GetActiveBookID() = %d, want 2", id)
	}
}
