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
