package store

import (
	"database/sql"
	"fmt"
)

// GetState retrieves a value from the state key-value table.
// Returns an empty string (and no error) when the key is not found.
func (s *Store) GetState(key string) (string, error) {
	const query = `SELECT value FROM state WHERE key = ?`
	var value string
	err := s.db.QueryRow(query, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get state %q: %w", key, err)
	}
	return value, nil
}

// SetState inserts or replaces a key-value pair in the state table.
func (s *Store) SetState(key, value string) error {
	const query = `INSERT OR REPLACE INTO state (key, value) VALUES (?, ?)`
	_, err := s.db.Exec(query, key, value)
	if err != nil {
		return fmt.Errorf("set state %q: %w", key, err)
	}
	return nil
}

// DeleteState removes a key from the state table.
func (s *Store) DeleteState(key string) error {
	_, err := s.db.Exec(`DELETE FROM state WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("delete state %q: %w", key, err)
	}
	return nil
}
