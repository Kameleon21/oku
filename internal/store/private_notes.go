package store

import "fmt"

// SavePrivateNote stores an editable note locally. It is independent of the
// synced library tables so refreshing or replacing a book cannot erase it.
func (s *Store) SavePrivateNote(bookID int, text string) error {
	if bookID <= 0 {
		return fmt.Errorf("invalid book id")
	}
	_, err := s.db.Exec(`INSERT INTO private_notes (book_id, text) VALUES (?, ?)
 ON CONFLICT(book_id) DO UPDATE SET text = excluded.text`, bookID, text)
	if err != nil {
		return fmt.Errorf("save private note: %w", err)
	}
	return nil
}

func (s *Store) ListPrivateNotes() (map[int]string, error) {
	rows, err := s.db.Query(`SELECT book_id, text FROM private_notes`)
	if err != nil {
		return nil, fmt.Errorf("list private notes: %w", err)
	}
	defer rows.Close()
	notes := map[int]string{}
	for rows.Next() {
		var id int
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, err
		}
		notes[id] = text
	}
	return notes, rows.Err()
}
