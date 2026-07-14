package store

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database connection for local book data.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at dbPath, runs migrations,
// and returns a ready-to-use Store.
func New(dbPath string) (*Store, error) {
	separator := "?"
	if strings.Contains(dbPath, "?") {
		separator = "&"
	}
	dsn := dbPath + separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// NOTE: do not SetMaxOpenConns(1) — several queries (e.g. ListUserBooks →
	// GetLatestRead) run while iterating another query's rows, which deadlocks
	// on a single-connection pool. busy_timeout above handles writer contention.

	// Enable WAL mode for better concurrent access.
	for _, pragma := range []string{"PRAGMA journal_mode=WAL"} {
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
	rating     REAL    NOT NULL DEFAULT 0,
	ratings_count INTEGER NOT NULL DEFAULT 0,
	reviews_count INTEGER NOT NULL DEFAULT 0,
	users_count INTEGER NOT NULL DEFAULT 0,
	users_read_count INTEGER NOT NULL DEFAULT 0,
	release_date TEXT   NOT NULL DEFAULT '',
	featured_series TEXT NOT NULL DEFAULT '',
	featured_series_position INTEGER NOT NULL DEFAULT 0,
	updated_at TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS user_books (
	id         INTEGER PRIMARY KEY,
	book_id    INTEGER NOT NULL,
	status_id  INTEGER NOT NULL DEFAULT 0,
	rating     REAL    NOT NULL DEFAULT 0,
	review     TEXT    NOT NULL DEFAULT '',
	reviewed_at TEXT,
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

CREATE TABLE IF NOT EXISTS reading_sessions (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	book_id    INTEGER DEFAULT 0,
	started_at TEXT NOT NULL,
	ended_at   TEXT,
	notes      TEXT DEFAULT ''
);
`
	if _, err := db.Exec(ddl); err != nil {
		return err
	}

	// Incremental migrations for existing installations.
	for _, col := range []struct {
		name string
		def  string
	}{
		{"rating", "REAL NOT NULL DEFAULT 0"},
		{"ratings_count", "INTEGER NOT NULL DEFAULT 0"},
		{"reviews_count", "INTEGER NOT NULL DEFAULT 0"},
		{"users_count", "INTEGER NOT NULL DEFAULT 0"},
		{"users_read_count", "INTEGER NOT NULL DEFAULT 0"},
		{"release_date", "TEXT NOT NULL DEFAULT ''"},
		{"featured_series", "TEXT NOT NULL DEFAULT ''"},
		{"featured_series_position", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := ensureColumn(db, "books", col.name, col.def); err != nil {
			return err
		}
	}
	for _, col := range []struct {
		name string
		def  string
	}{
		{"rating", "REAL NOT NULL DEFAULT 0"},
		{"review", "TEXT NOT NULL DEFAULT ''"},
		{"reviewed_at", "TEXT"},
	} {
		if err := ensureColumn(db, "user_books", col.name, col.def); err != nil {
			return err
		}
	}

	return nil
}

func ensureColumn(db *sql.DB, table, column, def string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notNull   int
			defaultV  sql.NullString
			primaryKV int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultV, &primaryKV); err != nil {
			return fmt.Errorf("scan table_info(%s): %w", table, err)
		}
		if strings.EqualFold(name, column) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate table_info(%s): %w", table, err)
	}

	stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, def)
	if _, err := db.Exec(stmt); err != nil {
		return fmt.Errorf("alter table %s add column %s: %w", table, column, err)
	}
	return nil
}
