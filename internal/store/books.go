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

// bookColumns are the books columns a model.Book is read from, in the order
// bookRow.dest expects. Queries alias the books table as b.
const bookColumns = `b.id, b.title, b.authors, b.pages, b.slug, b.image_url,
       b.rating, b.ratings_count, b.reviews_count, b.users_count, b.users_read_count,
       b.release_date, b.cached_tags, b.featured_series, b.featured_series_position`

// bookRow scans bookColumns into a model.Book. It contributes destinations to
// a larger row's Scan so a joined query reads the book the same way.
type bookRow struct {
	book    *model.Book
	authors string
}

func (r *bookRow) dest() []any {
	b := r.book
	return []any{
		&b.ID, &b.Title, &r.authors, &b.Pages, &b.Slug, &b.ImageURL,
		&b.Rating, &b.RatingsCount, &b.ReviewsCount, &b.UsersCount, &b.UsersReadCount,
		&b.ReleaseDate, &b.CachedTags, &b.FeaturedSeries, &b.FeaturedSeriesPosition,
	}
}

func (r *bookRow) finish() {
	r.book.Authors = splitAuthors(r.authors)
}

// rowScanner is satisfied by *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// GetBookByID retrieves a single book by its ID.
// Returns nil, nil when the book is not found.
func (s *Store) GetBookByID(id int) (*model.Book, error) {
	query := `SELECT ` + bookColumns + ` FROM books b WHERE b.id = ?`
	var b model.Book
	row := bookRow{book: &b}
	err := s.db.QueryRow(query, id).Scan(row.dest()...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get book %d: %w", id, err)
	}
	row.finish()
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
