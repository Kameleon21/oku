package store

import (
	"path/filepath"
	"testing"

	"github.com/Kameleon21/oku/internal/model"
)

func TestPrivateNotesSurviveReopenAndLibraryRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SavePrivateNote(42, "First thought"); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePrivateNote(42, "Revised thought\nSecond line"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUserBook(model.UserBook{ID: 7, BookID: 42, Book: model.Book{ID: 42, Title: "Book"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUserBook(model.UserBook{ID: 8, BookID: 42, Book: model.Book{ID: 42, Title: "Updated"}}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	notes, err := s.ListPrivateNotes()
	if err != nil || notes[42] != "Revised thought\nSecond line" {
		t.Fatalf("notes = %v, %v", notes, err)
	}
	if err := s.SavePrivateNote(42, ""); err != nil {
		t.Fatal(err)
	}
	notes, err = s.ListPrivateNotes()
	if err != nil || notes[42] != "" {
		t.Fatalf("clear note = %v, %v", notes, err)
	}
}

func TestPrivateNotesUpgradeFromVersionOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	seedFinishedRead(t, s, 1, 100, 4, "2026-06-01", "")
	if _, err := s.db.Exec(`DROP TABLE private_notes; PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SavePrivateNote(1, "After upgrade"); err != nil {
		t.Fatal(err)
	}
	summary, err := s.GetYearSummary(2026)
	if err != nil || summary.BooksFinished != 1 {
		t.Fatalf("upgrade lost reading history: %+v, %v", summary, err)
	}
	if got := userVersion(t, s); got != schemaVersion {
		t.Fatalf("version = %d", got)
	}
}
