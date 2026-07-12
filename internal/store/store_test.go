package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStateRoundTrip(t *testing.T) {
	s := testStore(t)

	// Empty key returns empty string.
	val, err := s.GetState("missing")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Fatalf("expected empty, got %q", val)
	}

	// Set and get.
	if err := s.SetState("key1", "value1"); err != nil {
		t.Fatal(err)
	}
	val, err = s.GetState("key1")
	if err != nil {
		t.Fatal(err)
	}
	if val != "value1" {
		t.Fatalf("expected value1, got %q", val)
	}

	// Overwrite.
	if err := s.SetState("key1", "value2"); err != nil {
		t.Fatal(err)
	}
	val, _ = s.GetState("key1")
	if val != "value2" {
		t.Fatalf("expected value2, got %q", val)
	}
}

func TestUpsertAndListBooks(t *testing.T) {
	s := testStore(t)
	reviewedAt := time.Now().UTC().Truncate(time.Second)

	ub := model.UserBook{
		ID:         100,
		BookID:     1,
		StatusID:   model.StatusCurrentlyReading,
		Rating:     4.5,
		Review:     "Strong recommendation",
		ReviewedAt: &reviewedAt,
		Book: model.Book{
			ID:      1,
			Title:   "Project Hail Mary",
			Authors: []string{"Andy Weir"},
			Pages:   476,
		},
		UpdatedAt: time.Now(),
	}

	if err := s.UpsertUserBook(ub); err != nil {
		t.Fatal(err)
	}

	// List by status.
	books, err := s.ListUserBooks(model.StatusCurrentlyReading)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}
	if books[0].Book.Title != "Project Hail Mary" {
		t.Fatalf("unexpected title: %s", books[0].Book.Title)
	}
	if len(books[0].Book.Authors) != 1 || books[0].Book.Authors[0] != "Andy Weir" {
		t.Fatalf("unexpected authors: %v", books[0].Book.Authors)
	}
	if books[0].Rating != 4.5 {
		t.Fatalf("expected rating 4.5, got %v", books[0].Rating)
	}
	if books[0].Review != "Strong recommendation" {
		t.Fatalf("unexpected review: %q", books[0].Review)
	}
	if books[0].ReviewedAt == nil {
		t.Fatal("expected reviewed_at to round-trip")
	}

	// Get by book ID.
	got, err := s.GetUserBookByBookID(1)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected book, got nil")
	}
	if got.ID != 100 {
		t.Fatalf("expected ID 100, got %d", got.ID)
	}

	// List wrong status returns empty.
	empty, err := s.ListUserBooks(model.StatusRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0, got %d", len(empty))
	}
}

func TestUserBookReadRoundTrip(t *testing.T) {
	s := testStore(t)

	// First create a book + user_book.
	ub := model.UserBook{
		ID:       200,
		BookID:   2,
		StatusID: model.StatusCurrentlyReading,
		Book: model.Book{
			ID:    2,
			Title: "Dune",
			Pages: 412,
		},
		UpdatedAt: time.Now(),
	}
	if err := s.UpsertUserBook(ub); err != nil {
		t.Fatal(err)
	}

	// No reads yet.
	r, err := s.GetLatestRead(200)
	if err != nil {
		t.Fatal(err)
	}
	if r != nil {
		t.Fatal("expected nil, got read")
	}

	// Insert a read.
	now := time.Now()
	read := model.UserBookRead{
		ID:            1,
		UserBookID:    200,
		ProgressPages: 50,
		StartedAt:     &now,
	}
	if err := s.UpsertUserBookRead(read); err != nil {
		t.Fatal(err)
	}

	r, err = s.GetLatestRead(200)
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected read, got nil")
	}
	if r.ProgressPages != 50 {
		t.Fatalf("expected 50, got %d", r.ProgressPages)
	}
}

func TestReplaceUserBooksForStatus(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	makeBook := func(userBookID, bookID int, title string) model.UserBook {
		return model.UserBook{
			ID:       userBookID,
			BookID:   bookID,
			StatusID: model.StatusCurrentlyReading,
			Book:     model.Book{ID: bookID, Title: title, Authors: []string{"Author"}},
			UserBookReads: []model.UserBookRead{
				{ID: userBookID*10 + 1, UserBookID: userBookID, ProgressPages: 10, StartedAt: &now},
			},
			UpdatedAt: now,
		}
	}

	// Seed with one book, plus a book in another status that must survive.
	if err := s.ReplaceUserBooksForStatus(model.StatusCurrentlyReading, []model.UserBook{makeBook(1, 100, "Old")}); err != nil {
		t.Fatal(err)
	}
	other := model.UserBook{
		ID: 9, BookID: 900, StatusID: model.StatusWantToRead,
		Book: model.Book{ID: 900, Title: "Other Status"}, UpdatedAt: now,
	}
	if err := s.UpsertUserBook(other); err != nil {
		t.Fatal(err)
	}

	// Replace with two different books.
	replacement := []model.UserBook{makeBook(2, 200, "New A"), makeBook(3, 300, "New B")}
	if err := s.ReplaceUserBooksForStatus(model.StatusCurrentlyReading, replacement); err != nil {
		t.Fatal(err)
	}

	books, err := s.ListUserBooks(model.StatusCurrentlyReading)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("got %d reading books, want 2", len(books))
	}
	for _, ub := range books {
		if ub.BookID == 100 {
			t.Fatal("old book survived the replace")
		}
		if len(ub.UserBookReads) == 0 {
			t.Fatalf("book %d lost its read row", ub.BookID)
		}
	}

	// Old book's reads must be gone; other status untouched.
	if r, err := s.GetLatestRead(1); err != nil || r != nil {
		t.Fatalf("GetLatestRead(1) = %v, %v; want nil, nil", r, err)
	}
	wanted, err := s.ListUserBooks(model.StatusWantToRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(wanted) != 1 {
		t.Fatalf("want-to-read books = %d, want 1 (other statuses untouched)", len(wanted))
	}
}

func TestUpsertUserBookCleansUpReplacedRowReads(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	ub := model.UserBook{
		ID: 1, BookID: 100, StatusID: model.StatusCurrentlyReading,
		Book: model.Book{ID: 100, Title: "Dune"}, UpdatedAt: now,
	}
	if err := s.UpsertUserBook(ub); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUserBookRead(model.UserBookRead{ID: 11, UserBookID: 1, ProgressPages: 42, StartedAt: &now}); err != nil {
		t.Fatal(err)
	}

	// Same book comes back with a new user_book id (removed and re-added
	// upstream). INSERT OR REPLACE deletes the old row on UNIQUE(book_id).
	ub.ID = 2
	if err := s.UpsertUserBook(ub); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetUserBookByBookID(100)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != 2 {
		t.Fatalf("GetUserBookByBookID(100) = %+v, want row with ID 2", got)
	}

	// The replaced row's reads must not linger as orphans.
	if r, err := s.GetLatestRead(1); err != nil || r != nil {
		t.Fatalf("GetLatestRead(1) = %v, %v; want nil, nil (orphan cleaned up)", r, err)
	}
}

func TestDBPathCreation(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub", "path")
	os.MkdirAll(subdir, 0o755)

	s, err := New(filepath.Join(subdir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
}
