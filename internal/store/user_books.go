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

// latestReadCTE ranks each user_book's reads, newest first, so the latest one
// can be joined in the same query instead of being fetched with a follow-up
// lookup per row. It is the only definition of "latest read".
const latestReadCTE = `
WITH latest_read AS (
	SELECT id, user_book_id, progress_pages, started_at, finished_at,
	       ROW_NUMBER() OVER (
	           PARTITION BY user_book_id ORDER BY started_at DESC, id DESC
	       ) AS rn
	FROM user_book_reads
)
`

// readColumns are the read columns readRow scans, aliased as r. They may come
// from a LEFT JOIN, so every one is nullable.
const readColumns = `r.id, r.progress_pages, r.started_at, r.finished_at`

// readRow scans readColumns.
type readRow struct {
	id, progressPages     sql.NullInt64
	startedAt, finishedAt sql.NullString
}

func (r *readRow) dest() []any {
	return []any{&r.id, &r.progressPages, &r.startedAt, &r.finishedAt}
}

// read returns the scanned read of userBookID, or nil when the row had none.
func (r *readRow) read(userBookID int) *model.UserBookRead {
	if !r.id.Valid {
		return nil
	}
	read := model.UserBookRead{
		ID:            int(r.id.Int64),
		UserBookID:    userBookID,
		ProgressPages: int(r.progressPages.Int64),
		StartedAt:     parseStoredTime(r.startedAt),
		FinishedAt:    parseStoredTime(r.finishedAt),
	}
	return &read
}

// parseStoredTime reads a timestamp the store wrote as RFC 3339. NULL and
// unreadable values are nil.
func parseStoredTime(v sql.NullString) *time.Time {
	if !v.Valid {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v.String)
	if err != nil {
		return nil
	}
	return &t
}

// userBookColumns are the columns scanUserBook reads: the user_book (ub), its
// book (b) and one of its reads (r).
const userBookColumns = `ub.id, ub.book_id, ub.status_id, ub.updated_at, ub.rating, ub.review, ub.reviewed_at,
       ` + bookColumns + `,
       ` + readColumns

// scanUserBook scans one row of userBookColumns. The read, when present,
// becomes the user book's only UserBookReads entry.
func scanUserBook(row rowScanner) (model.UserBook, error) {
	var ub model.UserBook
	var updatedAt string
	var reviewedAt sql.NullString
	book := bookRow{book: &ub.Book}
	var read readRow

	dest := []any{&ub.ID, &ub.BookID, &ub.StatusID, &updatedAt, &ub.Rating, &ub.Review, &reviewedAt}
	dest = append(dest, book.dest()...)
	dest = append(dest, read.dest()...)
	if err := row.Scan(dest...); err != nil {
		return model.UserBook{}, err
	}

	book.finish()
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		ub.UpdatedAt = t
	}
	ub.ReviewedAt = parseStoredTime(reviewedAt)
	if r := read.read(ub.ID); r != nil {
		ub.UserBookReads = []model.UserBookRead{*r}
	}
	return ub, nil
}

func scanUserBooks(rows *sql.Rows) ([]model.UserBook, error) {
	var result []model.UserBook
	for rows.Next() {
		ub, err := scanUserBook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user_book row: %w", err)
		}
		result = append(result, ub)
	}
	return result, rows.Err()
}

// ListUserBooks returns all user_books matching the given status, joined
// with the books table to populate the embedded Book struct and with each
// book's latest read to populate UserBookReads.
func (s *Store) ListUserBooks(status model.Status) ([]model.UserBook, error) {
	const query = latestReadCTE + `
SELECT ` + userBookColumns + `
FROM user_books ub
JOIN books b ON b.id = ub.book_id
LEFT JOIN latest_read r ON r.user_book_id = ub.id AND r.rn = 1
WHERE ub.status_id = ?
ORDER BY ub.updated_at DESC
`
	rows, err := s.db.Query(query, int(status))
	if err != nil {
		return nil, fmt.Errorf("list user_books status %d: %w", status, err)
	}
	defer rows.Close()

	return scanUserBooks(rows)
}

// GetUserBookByBookID retrieves a single user_book by its book_id, joined
// with the books table and its latest read. Returns nil, nil when not found.
func (s *Store) GetUserBookByBookID(bookID int) (*model.UserBook, error) {
	const query = latestReadCTE + `
SELECT ` + userBookColumns + `
FROM user_books ub
JOIN books b ON b.id = ub.book_id
LEFT JOIN latest_read r ON r.user_book_id = ub.id AND r.rn = 1
WHERE ub.book_id = ?
`
	ub, err := scanUserBook(s.db.QueryRow(query, bookID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user_book by book_id %d: %w", bookID, err)
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
// user_book_id, as ranked by latestReadCTE. Returns nil, nil when no reads
// exist.
func (s *Store) GetLatestRead(userBookID int) (*model.UserBookRead, error) {
	const query = latestReadCTE + `
SELECT ` + readColumns + `
FROM latest_read r
WHERE r.user_book_id = ? AND r.rn = 1
`
	var read readRow
	err := s.db.QueryRow(query, userBookID).Scan(read.dest()...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest read for user_book %d: %w", userBookID, err)
	}
	return read.read(userBookID), nil
}

// LibraryBookIDs returns the IDs of every book on any of the user's cached shelves.
func (s *Store) LibraryBookIDs() (map[int]bool, error) {
	rows, err := s.db.Query(`SELECT book_id FROM user_books`)
	if err != nil {
		return nil, fmt.Errorf("query library book ids: %w", err)
	}
	defer rows.Close()

	ids := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan library book id: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
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
