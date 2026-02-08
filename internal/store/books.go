package store

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/Kameleon21/oku/internal/model"
)

// UpsertBook inserts or replaces a book record.
// Authors are stored as a comma-separated string.
func (s *Store) UpsertBook(b model.Book) error {
	const query = `
INSERT OR REPLACE INTO books (id, title, authors, pages, slug, image_url, updated_at)
VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
`
	_, err := s.db.Exec(query, b.ID, b.Title, strings.Join(b.Authors, ", "), b.Pages, b.Slug, b.ImageURL)
	if err != nil {
		return fmt.Errorf("upsert book %d: %w", b.ID, err)
	}
	return nil
}

// GetBookByID retrieves a single book by its ID.
// Returns nil, nil when the book is not found.
func (s *Store) GetBookByID(id int) (*model.Book, error) {
	const query = `
SELECT id, title, authors, pages, slug, image_url
FROM books
WHERE id = ?
`
	var b model.Book
	var authors string
	err := s.db.QueryRow(query, id).Scan(&b.ID, &b.Title, &authors, &b.Pages, &b.Slug, &b.ImageURL)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get book %d: %w", id, err)
	}
	b.Authors = splitAuthors(authors)
	return &b, nil
}

// splitAuthors splits a comma-separated author string into a slice.
// An empty string produces an empty (nil) slice.
func splitAuthors(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ", ")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// PruneOrphanBooks removes cached books not referenced by user_books and older than maxAgeDays.
func (s *Store) PruneOrphanBooks(maxAgeDays int) error {
	if maxAgeDays <= 0 {
		maxAgeDays = 30
	}

	const query = `
DELETE FROM books
WHERE id NOT IN (SELECT book_id FROM user_books)
  AND updated_at < datetime('now', ?)
`
	age := fmt.Sprintf("-%d days", maxAgeDays)
	if _, err := s.db.Exec(query, age); err != nil {
		return fmt.Errorf("prune orphan books: %w", err)
	}
	return nil
}
