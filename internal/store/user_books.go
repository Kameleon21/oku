package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// UpsertUserBook inserts or replaces a user_book record and also upserts
// the associated book so the local books table stays in sync.
func (s *Store) UpsertUserBook(ub model.UserBook) error {
	if err := s.UpsertBook(ub.Book); err != nil {
		return err
	}

	const query = `
INSERT OR REPLACE INTO user_books (id, book_id, status_id, updated_at)
VALUES (?, ?, ?, ?)
`
	updatedAt := ub.UpdatedAt.UTC().Format(time.RFC3339)
	_, err := s.db.Exec(query, ub.ID, ub.BookID, int(ub.StatusID), updatedAt)
	if err != nil {
		return fmt.Errorf("upsert user_book %d: %w", ub.ID, err)
	}
	return nil
}

// ListUserBooks returns all user_books matching the given status, joined
// with the books table to populate the embedded Book struct.
func (s *Store) ListUserBooks(status model.Status) ([]model.UserBook, error) {
	const query = `
SELECT ub.id, ub.book_id, ub.status_id, ub.updated_at,
       b.id, b.title, b.authors, b.pages, b.slug, b.image_url
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
		var authors string
		err := rows.Scan(
			&ub.ID, &ub.BookID, &ub.StatusID, &updatedAt,
			&ub.Book.ID, &ub.Book.Title, &authors, &ub.Book.Pages, &ub.Book.Slug, &ub.Book.ImageURL,
		)
		if err != nil {
			return nil, fmt.Errorf("scan user_book row: %w", err)
		}
		ub.Book.Authors = splitAuthors(authors)
		if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
			ub.UpdatedAt = t
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
SELECT ub.id, ub.book_id, ub.status_id, ub.updated_at,
       b.id, b.title, b.authors, b.pages, b.slug, b.image_url
FROM user_books ub
JOIN books b ON b.id = ub.book_id
WHERE ub.book_id = ?
`
	var ub model.UserBook
	var updatedAt string
	var authors string
	err := s.db.QueryRow(query, bookID).Scan(
		&ub.ID, &ub.BookID, &ub.StatusID, &updatedAt,
		&ub.Book.ID, &ub.Book.Title, &authors, &ub.Book.Pages, &ub.Book.Slug, &ub.Book.ImageURL,
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

	// Attach latest reading progress.
	if read, err := s.GetLatestRead(ub.ID); err == nil && read != nil {
		ub.UserBookReads = []model.UserBookRead{*read}
	}
	return &ub, nil
}

// UpsertUserBookRead inserts or replaces a user_book_reads record.
func (s *Store) UpsertUserBookRead(r model.UserBookRead) error {
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

	_, err := s.db.Exec(query, r.ID, r.UserBookID, r.ProgressPages, startedAt, finishedAt)
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
