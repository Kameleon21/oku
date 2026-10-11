package store

import "fmt"

// ClearAccount drops everything cached for the signed-in Hardcover account,
// in one transaction: the library (user books and their reads, for every
// status), the synced and local journal entries, the goals, and the given
// state keys. Private notes, timer sessions, the running timer and the
// search history are device-local and deliberately kept: notes are keyed by
// book ID, so one written under one account shows again for the same book
// under another, and a running timer keeps running and is recorded as a
// session when stopped, whichever account is signed in.
func (s *Store) ClearAccount(stateKeys []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, q := range []string{
		`DELETE FROM user_book_reads`,
		`DELETE FROM user_books`,
		`DELETE FROM reading_journals`,
		`DELETE FROM goals`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("clear account: %w", err)
		}
	}
	for _, key := range stateKeys {
		if _, err := tx.Exec(`DELETE FROM state WHERE key = ?`, key); err != nil {
			return fmt.Errorf("clear account state %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit clear account: %w", err)
	}
	return nil
}
