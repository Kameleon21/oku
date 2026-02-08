package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database connection for local book data.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at dbPath, runs migrations,
// and returns a ready-to-use Store.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Enable WAL mode and foreign keys for better concurrent access.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("exec %s: %w", pragma, err)
		}
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate creates the schema tables if they do not already exist.
func migrate(db *sql.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS books (
	id         INTEGER PRIMARY KEY,
	title      TEXT    NOT NULL DEFAULT '',
	authors    TEXT    NOT NULL DEFAULT '',
	pages      INTEGER NOT NULL DEFAULT 0,
	slug       TEXT    NOT NULL DEFAULT '',
	image_url  TEXT    NOT NULL DEFAULT '',
	updated_at TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS user_books (
	id         INTEGER PRIMARY KEY,
	book_id    INTEGER NOT NULL,
	status_id  INTEGER NOT NULL DEFAULT 0,
	updated_at TEXT    NOT NULL DEFAULT '',
	UNIQUE(book_id)
);

CREATE TABLE IF NOT EXISTS user_book_reads (
	id              INTEGER PRIMARY KEY,
	user_book_id    INTEGER NOT NULL,
	progress_pages  INTEGER NOT NULL DEFAULT 0,
	started_at      TEXT,
	finished_at     TEXT
);

CREATE TABLE IF NOT EXISTS state (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL DEFAULT ''
);
`
	_, err := db.Exec(ddl)
	return err
}
