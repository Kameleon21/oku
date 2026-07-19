package model

import "time"

// ReadingSession represents a timed reading session stored locally.
type ReadingSession struct {
	ID        int
	BookID    int // 0 = no book associated
	StartedAt time.Time
	EndedAt   *time.Time // nil = still running (should not be stored without end)
	Notes     string
	BookTitle string // denormalized for display
}

// Duration returns the session duration. Returns zero if EndedAt is nil.
func (s ReadingSession) Duration() time.Duration {
	if s.EndedAt == nil {
		return 0
	}
	return s.EndedAt.Sub(s.StartedAt)
}

// TimerState represents the currently running timer, stored as JSON in the state KV table.
type TimerState struct {
	BookID    int       `json:"book_id"`
	StartedAt time.Time `json:"started_at"`
}

// DayActivity represents total reading minutes for a single day.
type DayActivity struct {
	Date        time.Time
	Minutes     int
	HasActivity bool // progress update or finished book that day (no timer minutes required)
}

// WeeklyStats holds aggregated stats for a time period.
type WeeklyStats struct {
	Days       [7]int // minutes per day (Mon=0 .. Sun=6)
	Total      int    // total minutes in the period
	Sessions   int    // total number of sessions
	LongestDay int    // index of day with most reading
}
