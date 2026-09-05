package store

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// schemaVersion is the current schema revision, tracked in PRAGMA user_version
// so migrations only run when the database is behind.
//
// Bump this whenever schemaDDL, its index list, or the ensureColumn lists in
// migrate change. A database already at the old value skips migrate entirely,
// so without a bump existing installations silently never get the change.
const schemaVersion = 1

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
	dsn := dbPath + separator + "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// NOTE: do not SetMaxOpenConns(1) — queries that run while iterating
	// another query's rows deadlock on a single-connection pool.
	// busy_timeout above handles writer contention.

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

// schemaDDL is the full schema: tables, the indexes covering the queries in
// this package, and the drop of tables retired from earlier revisions.
const schemaDDL = `
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
	cached_tags TEXT    NOT NULL DEFAULT '',
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

CREATE TABLE IF NOT EXISTS reading_journals (
	id        INTEGER PRIMARY KEY,
	action_at TEXT NOT NULL,
	event     TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS goals (
	id         INTEGER PRIMARY KEY,
	metric     TEXT NOT NULL DEFAULT '',
	target     INTEGER NOT NULL DEFAULT 0,
	progress   REAL NOT NULL DEFAULT 0,
	state      TEXT NOT NULL DEFAULT '',
	start_date TEXT,
	end_date   TEXT
);

DROP TABLE IF EXISTS activity_log;

CREATE INDEX IF NOT EXISTS idx_user_books_status_updated
	ON user_books(status_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_book_reads_latest
	ON user_book_reads(user_book_id, started_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_user_book_reads_finished_at
	ON user_book_reads(finished_at);
CREATE INDEX IF NOT EXISTS idx_reading_sessions_started_at
	ON reading_sessions(started_at);
CREATE INDEX IF NOT EXISTS idx_reading_sessions_book_id
	ON reading_sessions(book_id);
CREATE INDEX IF NOT EXISTS idx_reading_journals_action_at
	ON reading_journals(action_at);
`

// migrate creates the schema tables if they do not already exist. It is a
// no-op once PRAGMA user_version reports the database is already at
// schemaVersion, so repeated opens do not re-run the DDL.
func migrate(db *sql.DB) error {
	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if current >= schemaVersion {
		return nil
	}

	if _, err := db.Exec(schemaDDL); err != nil {
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
		{"cached_tags", "TEXT NOT NULL DEFAULT ''"},
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

	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
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
