package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStateRoundTrip(t *testing.T) {
	s := testStore(t)

	// Empty key returns empty string.
	val, err := s.GetState("missing")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Fatalf("expected empty, got %q", val)
	}

	// Set and get.
	if err := s.SetState("key1", "value1"); err != nil {
		t.Fatal(err)
	}
	val, err = s.GetState("key1")
	if err != nil {
		t.Fatal(err)
	}
	if val != "value1" {
		t.Fatalf("expected value1, got %q", val)
	}

	// Overwrite.
	if err := s.SetState("key1", "value2"); err != nil {
		t.Fatal(err)
	}
	val, _ = s.GetState("key1")
	if val != "value2" {
		t.Fatalf("expected value2, got %q", val)
	}
}

func TestUpsertAndListBooks(t *testing.T) {
	s := testStore(t)
	reviewedAt := time.Now().UTC().Truncate(time.Second)

	ub := model.UserBook{
		ID:         100,
		BookID:     1,
		StatusID:   model.StatusCurrentlyReading,
		Rating:     4.5,
		Review:     "Strong recommendation",
		ReviewedAt: &reviewedAt,
		Book: model.Book{
			ID:      1,
			Title:   "Project Hail Mary",
			Authors: []string{"Andy Weir"},
			Pages:   476,
		},
		UpdatedAt: time.Now(),
	}

	if err := s.UpsertUserBook(ub); err != nil {
		t.Fatal(err)
	}

	// List by status.
	books, err := s.ListUserBooks(model.StatusCurrentlyReading)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}
	if books[0].Book.Title != "Project Hail Mary" {
		t.Fatalf("unexpected title: %s", books[0].Book.Title)
	}
	if len(books[0].Book.Authors) != 1 || books[0].Book.Authors[0] != "Andy Weir" {
		t.Fatalf("unexpected authors: %v", books[0].Book.Authors)
	}
	if books[0].Rating != 4.5 {
		t.Fatalf("expected rating 4.5, got %v", books[0].Rating)
	}
	if books[0].Review != "Strong recommendation" {
		t.Fatalf("unexpected review: %q", books[0].Review)
	}
	if books[0].ReviewedAt == nil {
		t.Fatal("expected reviewed_at to round-trip")
	}

	// Get by book ID.
	got, err := s.GetUserBookByBookID(1)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected book, got nil")
	}
	if got.ID != 100 {
		t.Fatalf("expected ID 100, got %d", got.ID)
	}

	// List wrong status returns empty.
	empty, err := s.ListUserBooks(model.StatusRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected 0, got %d", len(empty))
	}
}

func TestUserBookReadRoundTrip(t *testing.T) {
	s := testStore(t)

	// First create a book + user_book.
	ub := model.UserBook{
		ID:       200,
		BookID:   2,
		StatusID: model.StatusCurrentlyReading,
		Book: model.Book{
			ID:    2,
			Title: "Dune",
			Pages: 412,
		},
		UpdatedAt: time.Now(),
	}
	if err := s.UpsertUserBook(ub); err != nil {
		t.Fatal(err)
	}

	// No reads yet.
	r, err := s.GetLatestRead(200)
	if err != nil {
		t.Fatal(err)
	}
	if r != nil {
		t.Fatal("expected nil, got read")
	}

	// Insert a read.
	now := time.Now()
	read := model.UserBookRead{
		ID:            1,
		UserBookID:    200,
		ProgressPages: 50,
		StartedAt:     &now,
	}
	if err := s.UpsertUserBookRead(read); err != nil {
		t.Fatal(err)
	}

	r, err = s.GetLatestRead(200)
	if err != nil {
		t.Fatal(err)
	}
	if r == nil {
		t.Fatal("expected read, got nil")
	}
	if r.ProgressPages != 50 {
		t.Fatalf("expected 50, got %d", r.ProgressPages)
	}
}

func TestReplaceUserBooksForStatus(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	makeBook := func(userBookID, bookID int, title string) model.UserBook {
		return model.UserBook{
			ID:       userBookID,
			BookID:   bookID,
			StatusID: model.StatusCurrentlyReading,
			Book:     model.Book{ID: bookID, Title: title, Authors: []string{"Author"}},
			UserBookReads: []model.UserBookRead{
				{ID: userBookID*10 + 1, UserBookID: userBookID, ProgressPages: 10, StartedAt: &now},
			},
			UpdatedAt: now,
		}
	}

	// Seed with one book, plus a book in another status that must survive.
	if err := s.ReplaceUserBooksForStatus(model.StatusCurrentlyReading, []model.UserBook{makeBook(1, 100, "Old")}); err != nil {
		t.Fatal(err)
	}
	other := model.UserBook{
		ID: 9, BookID: 900, StatusID: model.StatusWantToRead,
		Book: model.Book{ID: 900, Title: "Other Status"}, UpdatedAt: now,
	}
	if err := s.UpsertUserBook(other); err != nil {
		t.Fatal(err)
	}

	// Replace with two different books.
	replacement := []model.UserBook{makeBook(2, 200, "New A"), makeBook(3, 300, "New B")}
	if err := s.ReplaceUserBooksForStatus(model.StatusCurrentlyReading, replacement); err != nil {
		t.Fatal(err)
	}

	books, err := s.ListUserBooks(model.StatusCurrentlyReading)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("got %d reading books, want 2", len(books))
	}
	for _, ub := range books {
		if ub.BookID == 100 {
			t.Fatal("old book survived the replace")
		}
		if len(ub.UserBookReads) == 0 {
			t.Fatalf("book %d lost its read row", ub.BookID)
		}
	}

	// Old book's reads must be gone; other status untouched.
	if r, err := s.GetLatestRead(1); err != nil || r != nil {
		t.Fatalf("GetLatestRead(1) = %v, %v; want nil, nil", r, err)
	}
	wanted, err := s.ListUserBooks(model.StatusWantToRead)
	if err != nil {
		t.Fatal(err)
	}
	if len(wanted) != 1 {
		t.Fatalf("want-to-read books = %d, want 1 (other statuses untouched)", len(wanted))
	}
}

func TestUpsertUserBookCleansUpReplacedRowReads(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	ub := model.UserBook{
		ID: 1, BookID: 100, StatusID: model.StatusCurrentlyReading,
		Book: model.Book{ID: 100, Title: "Dune"}, UpdatedAt: now,
	}
	if err := s.UpsertUserBook(ub); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUserBookRead(model.UserBookRead{ID: 11, UserBookID: 1, ProgressPages: 42, StartedAt: &now}); err != nil {
		t.Fatal(err)
	}

	// Same book comes back with a new user_book id (removed and re-added
	// upstream). INSERT OR REPLACE deletes the old row on UNIQUE(book_id).
	ub.ID = 2
	if err := s.UpsertUserBook(ub); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetUserBookByBookID(100)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != 2 {
		t.Fatalf("GetUserBookByBookID(100) = %+v, want row with ID 2", got)
	}

	// The replaced row's reads must not linger as orphans.
	if r, err := s.GetLatestRead(1); err != nil || r != nil {
		t.Fatalf("GetLatestRead(1) = %v, %v; want nil, nil (orphan cleaned up)", r, err)
	}
}

func TestDBPathCreation(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub", "path")
	os.MkdirAll(subdir, 0o755)

	s, err := New(filepath.Join(subdir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
}

func TestMigrationRunsOnceAcrossOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migrate.db")

	s, err := New(path)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}
	if got := userVersion(t, s); got != schemaVersion {
		t.Fatalf("user_version after first open = %d, want %d", got, schemaVersion)
	}
	if err := s.SetState("key", "value"); err != nil {
		t.Fatal(err)
	}
	// Drop a table behind the migration's back. A second open must not
	// recreate it, which proves the DDL was skipped.
	if _, err := s.db.Exec(`DROP TABLE goals`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := New(path)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	if got := userVersion(t, s2); got != schemaVersion {
		t.Fatalf("user_version after second open = %d, want %d", got, schemaVersion)
	}
	if tableExists(t, s2, "goals") {
		t.Fatal("migration re-ran on a database already at schemaVersion")
	}
	if got, err := s2.GetState("key"); err != nil || got != "value" {
		t.Fatalf("GetState after reopen = %q, %v; want %q, nil", got, err, "value")
	}

	// An older database still reports user_version 0, so it takes the
	// migration path exactly once and comes back whole.
	if _, err := s2.db.Exec(`PRAGMA user_version = 0`); err != nil {
		t.Fatal(err)
	}
	if err := s2.Close(); err != nil {
		t.Fatal(err)
	}

	s3, err := New(path)
	if err != nil {
		t.Fatalf("third New: %v", err)
	}
	defer s3.Close()
	if got := userVersion(t, s3); got != schemaVersion {
		t.Fatalf("user_version after upgrade = %d, want %d", got, schemaVersion)
	}
	if !tableExists(t, s3, "goals") {
		t.Fatal("migration did not run on a database at user_version 0")
	}
	if got, err := s3.GetState("key"); err != nil || got != "value" {
		t.Fatalf("GetState after upgrade = %q, %v; want %q, nil", got, err, "value")
	}
}

func userVersion(t *testing.T, s *Store) int {
	t.Helper()
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	return v
}

func tableExists(t *testing.T, s *Store, name string) bool {
	t.Helper()
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	if err != nil {
		t.Fatalf("look up table %q: %v", name, err)
	}
	return n > 0
}

func TestListUserBooksAttachesLatestRead(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	older := now.Add(-48 * time.Hour)

	withReads := model.UserBook{
		ID: 1, BookID: 100, StatusID: model.StatusCurrentlyReading,
		Book: model.Book{ID: 100, Title: "Dune"}, UpdatedAt: now,
	}
	noReads := model.UserBook{
		ID: 2, BookID: 200, StatusID: model.StatusCurrentlyReading,
		Book: model.Book{ID: 200, Title: "Neuromancer"}, UpdatedAt: now.Add(-time.Hour),
	}
	for _, ub := range []model.UserBook{withReads, noReads} {
		if err := s.UpsertUserBook(ub); err != nil {
			t.Fatal(err)
		}
	}

	// The newest read carries the lower id, so a result ordered by id alone
	// would pick the wrong row.
	reads := []model.UserBookRead{
		{ID: 10, UserBookID: 1, ProgressPages: 180, StartedAt: &now},
		{ID: 11, UserBookID: 1, ProgressPages: 40, StartedAt: &older},
	}
	for _, r := range reads {
		if err := s.UpsertUserBookRead(r); err != nil {
			t.Fatal(err)
		}
	}

	books, err := s.ListUserBooks(model.StatusCurrentlyReading)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 {
		t.Fatalf("got %d books, want 2", len(books))
	}
	byID := make(map[int]model.UserBook, len(books))
	for _, ub := range books {
		byID[ub.ID] = ub
	}

	got := byID[1]
	if len(got.UserBookReads) != 1 {
		t.Fatalf("user_book 1 reads = %d, want 1", len(got.UserBookReads))
	}
	latest := got.UserBookReads[0]
	if latest.ID != 10 || latest.ProgressPages != 180 || latest.UserBookID != 1 {
		t.Fatalf("latest read = %+v, want id 10 with 180 pages for user_book 1", latest)
	}
	if latest.StartedAt == nil || !latest.StartedAt.Equal(now) {
		t.Fatalf("latest read started_at = %v, want %v", latest.StartedAt, now)
	}
	if latest.FinishedAt != nil {
		t.Fatalf("latest read finished_at = %v, want nil", latest.FinishedAt)
	}

	// The joined row must match the standalone lookup exactly.
	direct, err := s.GetLatestRead(1)
	if err != nil {
		t.Fatal(err)
	}
	if direct == nil || direct.ID != latest.ID || direct.ProgressPages != latest.ProgressPages {
		t.Fatalf("GetLatestRead(1) = %+v, want %+v", direct, latest)
	}

	if len(byID[2].UserBookReads) != 0 {
		t.Fatalf("user_book 2 reads = %+v, want none", byID[2].UserBookReads)
	}

	// GetUserBookByBookID uses the same join.
	single, err := s.GetUserBookByBookID(100)
	if err != nil {
		t.Fatal(err)
	}
	if single == nil || len(single.UserBookReads) != 1 || single.UserBookReads[0].ID != 10 {
		t.Fatalf("GetUserBookByBookID(100) reads = %+v, want the id 10 read", single)
	}
	bare, err := s.GetUserBookByBookID(200)
	if err != nil {
		t.Fatal(err)
	}
	if bare == nil || len(bare.UserBookReads) != 0 {
		t.Fatalf("GetUserBookByBookID(200) reads = %+v, want none", bare)
	}
}

func TestPruneOrphanBooksKeepsBooksWithSessions(t *testing.T) {
	s := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	for _, b := range []model.Book{
		{ID: 1, Title: "Shelved"},
		{ID: 2, Title: "Timed"},
		{ID: 3, Title: "Orphan"},
	} {
		if err := s.UpsertBook(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.UpsertUserBook(model.UserBook{
		ID: 1, BookID: 1, StatusID: model.StatusRead,
		Book: model.Book{ID: 1, Title: "Shelved"}, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	ended := now.Add(time.Hour)
	if _, err := s.InsertSession(model.ReadingSession{BookID: 2, StartedAt: now, EndedAt: &ended}); err != nil {
		t.Fatal(err)
	}
	// A session with no book must not turn the NOT IN predicate into NULL and
	// silently cancel the whole prune.
	if _, err := s.db.Exec(
		`INSERT INTO reading_sessions (book_id, started_at, ended_at) VALUES (NULL, ?, ?)`,
		now.Format(time.RFC3339), ended.Format(time.RFC3339),
	); err != nil {
		t.Fatal(err)
	}

	// Age every cached book past the prune window (upserts stamp 'now').
	if _, err := s.db.Exec(`UPDATE books SET updated_at = datetime('now', '-90 days')`); err != nil {
		t.Fatal(err)
	}

	if err := s.PruneOrphanBooks(30); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		id     int
		reason string
		keep   bool
	}{
		{1, "referenced by user_books", true},
		{2, "referenced by a reading session", true},
		{3, "unreferenced", false},
	} {
		b, err := s.GetBookByID(tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if tc.keep && b == nil {
			t.Fatalf("book %d (%s) was pruned", tc.id, tc.reason)
		}
		if !tc.keep && b != nil {
			t.Fatalf("book %d (%s) survived the prune", tc.id, tc.reason)
		}
	}
}

// legacySchemaDDL is the schema as it stood before indexes, the cached_tags
// column and the activity_log drop landed: the shape a user upgrading from an
// older release actually has on disk.
const legacySchemaDDL = `
CREATE TABLE books (
	id INTEGER PRIMARY KEY,
	title TEXT NOT NULL DEFAULT '',
	authors TEXT NOT NULL DEFAULT '',
	pages INTEGER NOT NULL DEFAULT 0,
	slug TEXT NOT NULL DEFAULT '',
	image_url TEXT NOT NULL DEFAULT '',
	rating REAL NOT NULL DEFAULT 0,
	ratings_count INTEGER NOT NULL DEFAULT 0,
	reviews_count INTEGER NOT NULL DEFAULT 0,
	users_count INTEGER NOT NULL DEFAULT 0,
	users_read_count INTEGER NOT NULL DEFAULT 0,
	release_date TEXT NOT NULL DEFAULT '',
	featured_series TEXT NOT NULL DEFAULT '',
	featured_series_position INTEGER NOT NULL DEFAULT 0,
	updated_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE user_books (
	id INTEGER PRIMARY KEY,
	book_id INTEGER NOT NULL,
	status_id INTEGER NOT NULL DEFAULT 0,
	rating REAL NOT NULL DEFAULT 0,
	review TEXT NOT NULL DEFAULT '',
	reviewed_at TEXT,
	updated_at TEXT NOT NULL DEFAULT '',
	UNIQUE(book_id)
);
CREATE TABLE user_book_reads (
	id INTEGER PRIMARY KEY,
	user_book_id INTEGER NOT NULL,
	progress_pages INTEGER NOT NULL DEFAULT 0,
	started_at TEXT,
	finished_at TEXT
);
CREATE TABLE state (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL DEFAULT ''
);
CREATE TABLE reading_sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	book_id INTEGER DEFAULT 0,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	notes TEXT DEFAULT ''
);
CREATE TABLE reading_journals (
	id INTEGER PRIMARY KEY,
	action_at TEXT NOT NULL,
	event TEXT NOT NULL DEFAULT ''
);
CREATE TABLE goals (
	id INTEGER PRIMARY KEY,
	metric TEXT NOT NULL DEFAULT '',
	target INTEGER NOT NULL DEFAULT 0,
	progress REAL NOT NULL DEFAULT 0,
	state TEXT NOT NULL DEFAULT '',
	start_date TEXT,
	end_date TEXT
);
CREATE TABLE activity_log (
	id INTEGER PRIMARY KEY,
	event TEXT NOT NULL DEFAULT ''
);
INSERT INTO books (id, title, updated_at) VALUES (7, 'Legacy', '2020-01-01T00:00:00Z');
INSERT INTO state (key, value) VALUES ('legacy', 'kept');
`

func TestMigrationUpgradesLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy fixture: %v", err)
	}
	if _, err := raw.Exec(legacySchemaDDL); err != nil {
		t.Fatalf("build legacy fixture: %v", err)
	}
	var version int
	if err := raw.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("legacy fixture user_version = %d, want 0", version)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := New(path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer s.Close()

	if !columnExists(t, s, "books", "cached_tags") {
		t.Fatal("books.cached_tags was not added to the legacy database")
	}
	// Compare against the names in schemaDDL so an index added later is
	// checked here automatically.
	for _, name := range ddlIndexNames(t) {
		if !indexExists(t, s, name) {
			t.Fatalf("index %q from the DDL is missing after the upgrade", name)
		}
	}
	if tableExists(t, s, "activity_log") {
		t.Fatal("activity_log survived the upgrade")
	}
	if got := userVersion(t, s); got != schemaVersion {
		t.Fatalf("user_version after upgrade = %d, want %d", got, schemaVersion)
	}

	// The upgrade must not cost the user their cache.
	b, err := s.GetBookByID(7)
	if err != nil {
		t.Fatal(err)
	}
	if b == nil || b.Title != "Legacy" {
		t.Fatalf("GetBookByID(7) = %+v, want the pre-existing row", b)
	}
	if got, err := s.GetState("legacy"); err != nil || got != "kept" {
		t.Fatalf("GetState(\"legacy\") = %q, %v; want %q, nil", got, err, "kept")
	}
}

// ddlIndexNames extracts the index names declared in schemaDDL.
func ddlIndexNames(t *testing.T) []string {
	t.Helper()
	const prefix = "CREATE INDEX IF NOT EXISTS "

	var names []string
	for _, line := range strings.Split(schemaDDL, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		name := strings.TrimPrefix(line, prefix)
		if i := strings.IndexAny(name, " \t("); i >= 0 {
			name = name[:i]
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		t.Fatal("no CREATE INDEX statements found in schemaDDL")
	}
	return names
}

func indexExists(t *testing.T, s *Store, name string) bool {
	t.Helper()
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name,
	).Scan(&n)
	if err != nil {
		t.Fatalf("look up index %q: %v", name, err)
	}
	return n > 0
}

func columnExists(t *testing.T, s *Store, table, column string) bool {
	t.Helper()
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatalf("table_info(%s): %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if strings.EqualFold(name, column) {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}
