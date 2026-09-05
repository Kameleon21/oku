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
	return upsertBook(s.db, b)
}

func upsertBook(e execer, b model.Book) error {
	const query = `
INSERT OR REPLACE INTO books (
	id, title, authors, pages, slug, image_url,
	rating, ratings_count, reviews_count, users_count, users_read_count,
	release_date, cached_tags, featured_series, featured_series_position, updated_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
`
	_, err := e.Exec(
		query,
		b.ID, b.Title, strings.Join(b.Authors, ", "), b.Pages, b.Slug, b.ImageURL,
		b.Rating, b.RatingsCount, b.ReviewsCount, b.UsersCount, b.UsersReadCount,
		b.ReleaseDate, b.CachedTags, b.FeaturedSeries, b.FeaturedSeriesPosition,
	)
	if err != nil {
		return fmt.Errorf("upsert book %d: %w", b.ID, err)
	}
	return nil
}

// GetBookByID retrieves a single book by its ID.
// Returns nil, nil when the book is not found.
func (s *Store) GetBookByID(id int) (*model.Book, error) {
	const query = `
SELECT id, title, authors, pages, slug, image_url,
       rating, ratings_count, reviews_count, users_count, users_read_count,
       release_date, featured_series, featured_series_position
FROM books
WHERE id = ?
`
	var b model.Book
	var authors string
	err := s.db.QueryRow(query, id).Scan(
		&b.ID, &b.Title, &authors, &b.Pages, &b.Slug, &b.ImageURL,
		&b.Rating, &b.RatingsCount, &b.ReviewsCount, &b.UsersCount, &b.UsersReadCount,
		&b.ReleaseDate, &b.FeaturedSeries, &b.FeaturedSeriesPosition,
	)
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

// PruneOrphanBooks removes cached books older than maxAgeDays that are
// referenced neither by user_books nor by reading_sessions, so timer history
// keeps its book titles.
func (s *Store) PruneOrphanBooks(maxAgeDays int) error {
	if maxAgeDays <= 0 {
		maxAgeDays = 30
	}

	// The IS NOT NULL guards matter: a single NULL in a NOT IN subquery makes
	// the whole predicate NULL and silently prunes nothing.
	const query = `
DELETE FROM books
WHERE id NOT IN (SELECT book_id FROM user_books WHERE book_id IS NOT NULL)
  AND id NOT IN (SELECT book_id FROM reading_sessions WHERE book_id IS NOT NULL)
  AND updated_at < datetime('now', ?)
`
	age := fmt.Sprintf("-%d days", maxAgeDays)
	if _, err := s.db.Exec(query, age); err != nil {
		return fmt.Errorf("prune orphan books: %w", err)
	}
	return nil
}
