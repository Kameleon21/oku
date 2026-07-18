package store

import (
	"fmt"
	"time"
)

// InsertActivity records a local activity event (progress update or book finished).
func (s *Store) InsertActivity(bookID int, event string, at time.Time) error {
	const query = `
INSERT INTO activity_log (book_id, event, created_at)
VALUES (?, ?, ?)
`
	if _, err := s.db.Exec(query, bookID, event, at.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("insert activity: %w", err)
	}
	return nil
}

// GetActivityDays returns the distinct local-time days with at least one
// activity event in the given range.
func (s *Store) GetActivityDays(from, to time.Time) ([]time.Time, error) {
	const query = `
SELECT DISTINCT date(created_at, 'localtime') as day
FROM activity_log
WHERE date(created_at, 'localtime') >= date(?)
  AND date(created_at, 'localtime') <= date(?)
ORDER BY day
`
	rows, err := s.db.Query(query, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("get activity days: %w", err)
	}
	defer rows.Close()

	var days []time.Time
	for rows.Next() {
		var dayStr string
		if err := rows.Scan(&dayStr); err != nil {
			return nil, fmt.Errorf("scan activity day: %w", err)
		}
		if t, err := time.ParseInLocation("2006-01-02", dayStr, time.Local); err == nil {
			days = append(days, t)
		}
	}
	return days, rows.Err()
}
