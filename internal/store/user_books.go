package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// execer is satisfied by both *sql.DB and *sql.Tx so upserts can run
// standalone or inside a transaction.
type execer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// UpsertUserBook inserts or replaces a user_book record and also upserts
// the associated book so the local books table stays in sync.
func (s *Store) UpsertUserBook(ub model.UserBook) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := upsertUserBook(tx, ub); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert user_book %d: %w", ub.ID, err)
	}
	return nil
}

func upsertUserBook(e execer, ub model.UserBook) error {
	if err := upsertBook(e, ub.Book); err != nil {
		return err
	}

	// INSERT OR REPLACE resolves the UNIQUE(book_id) conflict by deleting the
	// old row (which may have a different id); delete its reads first so they
	// are not left orphaned.
	const cleanupReads = `
DELETE FROM user_book_reads
WHERE user_book_id IN (
	SELECT id FROM user_books WHERE book_id = ? AND id != ?
)
`
	if _, err := e.Exec(cleanupReads, ub.BookID, ub.ID); err != nil {
		return fmt.Errorf("cleanup reads for book %d: %w", ub.BookID, err)
	}

	const query = `
INSERT OR REPLACE INTO user_books (id, book_id, status_id, rating, review, reviewed_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`
	updatedAt := ub.UpdatedAt.UTC().Format(time.RFC3339)
	var reviewedAt *string
	if ub.ReviewedAt != nil {
		formatted := ub.ReviewedAt.UTC().Format(time.RFC3339)
		reviewedAt = &formatted
	}
	_, err := e.Exec(
		query,
		ub.ID,
		ub.BookID,
		int(ub.StatusID),
		ub.Rating,
		ub.Review,
		reviewedAt,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert user_book %d: %w", ub.ID, err)
	}
	return nil
}

// ListUserBooks returns all user_books matching the given status, joined
// with the books table to populate the embedded Book struct.
func (s *Store) ListUserBooks(status model.Status) ([]model.UserBook, error) {
	const query = `
	SELECT ub.id, ub.book_id, ub.status_id, ub.updated_at, ub.rating, ub.review, ub.reviewed_at,
	       b.id, b.title, b.authors, b.pages, b.slug, b.image_url,
	       b.rating, b.ratings_count, b.reviews_count, b.users_count, b.users_read_count,
	       b.release_date, b.featured_series, b.featured_series_position
FROM user_books ub
JOIN books b ON b.id = ub.book_id
WHERE ub.status_id = ?
ORDER BY ub.updated_at DESC
`
	rows, err := s.db.Query(query, int(status))
	if err != nil {
		return nil, fmt.Errorf("list user_books status %d: %w", status, err)
	}
	defer rows.Close()

	var result []model.UserBook
	for rows.Next() {
		var ub model.UserBook
		var updatedAt string
		var reviewedAt sql.NullString
		var authors string
		err := rows.Scan(
			&ub.ID, &ub.BookID, &ub.StatusID, &updatedAt, &ub.Rating, &ub.Review, &reviewedAt,
			&ub.Book.ID, &ub.Book.Title, &authors, &ub.Book.Pages, &ub.Book.Slug, &ub.Book.ImageURL,
			&ub.Book.Rating, &ub.Book.RatingsCount, &ub.Book.ReviewsCount, &ub.Book.UsersCount, &ub.Book.UsersReadCount,
			&ub.Book.ReleaseDate, &ub.Book.FeaturedSeries, &ub.Book.FeaturedSeriesPosition,
		)
		if err != nil {
			return nil, fmt.Errorf("scan user_book row: %w", err)
		}
		ub.Book.Authors = splitAuthors(authors)
		if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
			ub.UpdatedAt = t
		}
		if reviewedAt.Valid {
			if t, err := time.Parse(time.RFC3339, reviewedAt.String); err == nil {
				ub.ReviewedAt = &t
			}
		}
		// Attach latest reading progress.
		if read, err := s.GetLatestRead(ub.ID); err == nil && read != nil {
			ub.UserBookReads = []model.UserBookRead{*read}
		}
		result = append(result, ub)
	}
	return result, rows.Err()
}

// GetUserBookByBookID retrieves a single user_book by its book_id, joined
// with the books table. Returns nil, nil when not found.
func (s *Store) GetUserBookByBookID(bookID int) (*model.UserBook, error) {
	const query = `
	SELECT ub.id, ub.book_id, ub.status_id, ub.updated_at, ub.rating, ub.review, ub.reviewed_at,
	       b.id, b.title, b.authors, b.pages, b.slug, b.image_url,
	       b.rating, b.ratings_count, b.reviews_count, b.users_count, b.users_read_count,
	       b.release_date, b.featured_series, b.featured_series_position
FROM user_books ub
JOIN books b ON b.id = ub.book_id
WHERE ub.book_id = ?
`
	var ub model.UserBook
	var updatedAt string
	var reviewedAt sql.NullString
	var authors string
	err := s.db.QueryRow(query, bookID).Scan(
		&ub.ID, &ub.BookID, &ub.StatusID, &updatedAt, &ub.Rating, &ub.Review, &reviewedAt,
		&ub.Book.ID, &ub.Book.Title, &authors, &ub.Book.Pages, &ub.Book.Slug, &ub.Book.ImageURL,
		&ub.Book.Rating, &ub.Book.RatingsCount, &ub.Book.ReviewsCount, &ub.Book.UsersCount, &ub.Book.UsersReadCount,
		&ub.Book.ReleaseDate, &ub.Book.FeaturedSeries, &ub.Book.FeaturedSeriesPosition,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user_book by book_id %d: %w", bookID, err)
	}
	ub.Book.Authors = splitAuthors(authors)
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		ub.UpdatedAt = t
	}
	if reviewedAt.Valid {
		if t, err := time.Parse(time.RFC3339, reviewedAt.String); err == nil {
			ub.ReviewedAt = &t
		}
	}

	// Attach latest reading progress.
	if read, err := s.GetLatestRead(ub.ID); err == nil && read != nil {
		ub.UserBookReads = []model.UserBookRead{*read}
	}
	return &ub, nil
}

// UpsertUserBookRead inserts or replaces a user_book_reads record.
func (s *Store) UpsertUserBookRead(r model.UserBookRead) error {
	return upsertUserBookRead(s.db, r)
}

func upsertUserBookRead(e execer, r model.UserBookRead) error {
	const query = `
INSERT OR REPLACE INTO user_book_reads (id, user_book_id, progress_pages, started_at, finished_at)
VALUES (?, ?, ?, ?, ?)
`
	var startedAt, finishedAt *string
	if r.StartedAt != nil {
		s := r.StartedAt.UTC().Format(time.RFC3339)
		startedAt = &s
	}
	if r.FinishedAt != nil {
		s := r.FinishedAt.UTC().Format(time.RFC3339)
		finishedAt = &s
	}

	_, err := e.Exec(query, r.ID, r.UserBookID, r.ProgressPages, startedAt, finishedAt)
	if err != nil {
		return fmt.Errorf("upsert user_book_read %d: %w", r.ID, err)
	}
	return nil
}

// GetLatestRead returns the most recent user_book_reads row for the given
// user_book_id, ordered by started_at descending. Returns nil, nil when
// no reads exist.
func (s *Store) GetLatestRead(userBookID int) (*model.UserBookRead, error) {
	const query = `
SELECT id, user_book_id, progress_pages, started_at, finished_at
FROM user_book_reads
WHERE user_book_id = ?
ORDER BY started_at DESC, id DESC
LIMIT 1
`
	var r model.UserBookRead
	var startedAt, finishedAt sql.NullString
	err := s.db.QueryRow(query, userBookID).Scan(
		&r.ID, &r.UserBookID, &r.ProgressPages, &startedAt, &finishedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest read for user_book %d: %w", userBookID, err)
	}
	if startedAt.Valid {
		if t, err := time.Parse(time.RFC3339, startedAt.String); err == nil {
			r.StartedAt = &t
		}
	}
	if finishedAt.Valid {
		if t, err := time.Parse(time.RFC3339, finishedAt.String); err == nil {
			r.FinishedAt = &t
		}
	}
	return &r, nil
}

// DeleteUserBooksByStatus removes all user_books rows for a status and their reads.
func (s *Store) DeleteUserBooksByStatus(status model.Status) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	const deleteReads = `
DELETE FROM user_book_reads
WHERE user_book_id IN (
	SELECT id FROM user_books WHERE status_id = ?
)
`
	if _, err := tx.Exec(deleteReads, int(status)); err != nil {
		return fmt.Errorf("delete reads by status %d: %w", status, err)
	}

	const deleteBooks = `DELETE FROM user_books WHERE status_id = ?`
	if _, err := tx.Exec(deleteBooks, int(status)); err != nil {
		return fmt.Errorf("delete user_books status %d: %w", status, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete status %d: %w", status, err)
	}
	return nil
}

// ReplaceUserBooksForStatus atomically swaps all cached rows for a status
// with the given books (and their reads). A failure mid-way rolls back,
// leaving the previous cache intact.
func (s *Store) ReplaceUserBooksForStatus(status model.Status, books []model.UserBook) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	const deleteReads = `
DELETE FROM user_book_reads
WHERE user_book_id IN (
	SELECT id FROM user_books WHERE status_id = ?
)
`
	if _, err := tx.Exec(deleteReads, int(status)); err != nil {
		return fmt.Errorf("delete reads by status %d: %w", status, err)
	}

	const deleteBooks = `DELETE FROM user_books WHERE status_id = ?`
	if _, err := tx.Exec(deleteBooks, int(status)); err != nil {
		return fmt.Errorf("delete user_books status %d: %w", status, err)
	}

	for _, ub := range books {
		if err := upsertUserBook(tx, ub); err != nil {
			return err
		}
		for _, r := range ub.UserBookReads {
			if err := upsertUserBookRead(tx, r); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace status %d: %w", status, err)
	}
	return nil
}
