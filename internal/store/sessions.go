package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// InsertSession inserts a completed reading session and returns its ID.
func (s *Store) InsertSession(session model.ReadingSession) (int, error) {
	const query = `
INSERT INTO reading_sessions (book_id, started_at, ended_at, notes)
VALUES (?, ?, ?, ?)
`
	startedAt := session.StartedAt.UTC().Format(time.RFC3339)
	var endedAt *string
	if session.EndedAt != nil {
		v := session.EndedAt.UTC().Format(time.RFC3339)
		endedAt = &v
	}

	result, err := s.db.Exec(query, session.BookID, startedAt, endedAt, session.Notes)
	if err != nil {
		return 0, fmt.Errorf("insert session: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get session id: %w", err)
	}
	return int(id), nil
}

// ListSessions returns the most recent reading sessions, limited to `limit`.
// Sessions are joined with books to populate BookTitle when available.
func (s *Store) ListSessions(limit int) ([]model.ReadingSession, error) {
	if limit <= 0 {
		limit = 10
	}

	const query = `
SELECT rs.id, rs.book_id, rs.started_at, rs.ended_at, rs.notes,
       COALESCE(b.title, '')
FROM reading_sessions rs
LEFT JOIN books b ON b.id = rs.book_id
WHERE rs.ended_at IS NOT NULL
ORDER BY rs.started_at DESC
LIMIT ?
`
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []model.ReadingSession
	for rows.Next() {
		var rs model.ReadingSession
		var startedAt string
		var endedAt sql.NullString
		err := rows.Scan(&rs.ID, &rs.BookID, &startedAt, &endedAt, &rs.Notes, &rs.BookTitle)
		if err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		if t, err := time.Parse(time.RFC3339, startedAt); err == nil {
			rs.StartedAt = t
		}
		if endedAt.Valid {
			if t, err := time.Parse(time.RFC3339, endedAt.String); err == nil {
				rs.EndedAt = &t
			}
		}
		sessions = append(sessions, rs)
	}
	return sessions, rows.Err()
}

// GetDailyActivity returns total reading minutes per day in the given range.
// Only completed sessions (ended_at IS NOT NULL) are counted.
func (s *Store) GetDailyActivity(from, to time.Time) ([]model.DayActivity, error) {
	const query = `
SELECT date(started_at) as day,
       SUM(
           CAST((julianday(ended_at) - julianday(started_at)) * 1440 AS INTEGER)
       ) as minutes
FROM reading_sessions
WHERE ended_at IS NOT NULL
  AND date(started_at) >= date(?)
  AND date(started_at) <= date(?)
GROUP BY date(started_at)
ORDER BY day
`
	fromStr := from.Format("2006-01-02")
	toStr := to.Format("2006-01-02")

	rows, err := s.db.Query(query, fromStr, toStr)
	if err != nil {
		return nil, fmt.Errorf("get daily activity: %w", err)
	}
	defer rows.Close()

	var activities []model.DayActivity
	for rows.Next() {
		var dayStr string
		var minutes int
		if err := rows.Scan(&dayStr, &minutes); err != nil {
			return nil, fmt.Errorf("scan daily activity: %w", err)
		}
		if t, err := time.Parse("2006-01-02", dayStr); err == nil {
			if minutes < 0 {
				minutes = 0
			}
			activities = append(activities, model.DayActivity{Date: t, Minutes: minutes})
		}
	}
	return activities, rows.Err()
}

// GetWeeklyStats returns aggregated stats for completed sessions in the given range.
func (s *Store) GetWeeklyStats(from, to time.Time) (model.WeeklyStats, error) {
	const query = `
SELECT started_at, ended_at
FROM reading_sessions
WHERE ended_at IS NOT NULL
  AND date(started_at) >= date(?)
  AND date(started_at) <= date(?)
ORDER BY started_at
`
	fromStr := from.Format("2006-01-02")
	toStr := to.Format("2006-01-02")

	rows, err := s.db.Query(query, fromStr, toStr)
	if err != nil {
		return model.WeeklyStats{}, fmt.Errorf("get weekly stats: %w", err)
	}
	defer rows.Close()

	var stats model.WeeklyStats
	for rows.Next() {
		var startStr, endStr string
		if err := rows.Scan(&startStr, &endStr); err != nil {
			return model.WeeklyStats{}, fmt.Errorf("scan weekly stats: %w", err)
		}

		startTime, err1 := time.Parse(time.RFC3339, startStr)
		endTime, err2 := time.Parse(time.RFC3339, endStr)
		if err1 != nil || err2 != nil {
			continue
		}

		minutes := int(endTime.Sub(startTime).Minutes())
		if minutes < 0 {
			continue
		}

		// time.Weekday: Sun=0, Mon=1, ..., Sat=6
		// We want Mon=0, ..., Sun=6
		dayIdx := (int(startTime.Weekday()) + 6) % 7
		stats.Days[dayIdx] += minutes
		stats.Total += minutes
		stats.Sessions++
	}

	// Find longest day
	for i, m := range stats.Days {
		if m > stats.Days[stats.LongestDay] {
			stats.LongestDay = i
		}
	}

	return stats, rows.Err()
}

// DeleteState removes a key from the state table.
func (s *Store) DeleteState(key string) error {
	_, err := s.db.Exec(`DELETE FROM state WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("delete state %q: %w", key, err)
	}
	return nil
}
