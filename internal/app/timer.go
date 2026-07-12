package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

const activeTimerKey = "active_timer"

// TimerStart begins a new reading session for the given book.
// Returns an error if a timer is already running.
func (a *App) TimerStart(bookID int) error {
	existing, err := a.TimerStatus()
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("timer already running (started %s)", existing.StartedAt.Local().Format("3:04 PM"))
	}

	state := model.TimerState{
		BookID:    bookID,
		StartedAt: time.Now().UTC(),
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal timer state: %w", err)
	}
	return a.Store.SetState(activeTimerKey, string(raw))
}

// TimerStop ends the current timer, records the session, and returns it.
// Returns an error if no timer is running.
func (a *App) TimerStop() (*model.ReadingSession, error) {
	state, err := a.TimerStatus()
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, fmt.Errorf("no timer running")
	}

	now := time.Now().UTC()
	session := model.ReadingSession{
		BookID:    state.BookID,
		StartedAt: state.StartedAt,
		EndedAt:   &now,
	}

	// Look up book title for display.
	if state.BookID > 0 {
		if b, err := a.Store.GetBookByID(state.BookID); err == nil && b != nil {
			session.BookTitle = b.Title
		}
	}

	id, err := a.Store.InsertSession(session)
	if err != nil {
		return nil, fmt.Errorf("save session: %w", err)
	}
	session.ID = id

	if err := a.Store.DeleteState(activeTimerKey); err != nil {
		return nil, fmt.Errorf("clear timer state: %w", err)
	}

	return &session, nil
}

// TimerStatus returns the current timer state, or nil if no timer is running.
func (a *App) TimerStatus() (*model.TimerState, error) {
	raw, err := a.Store.GetState(activeTimerKey)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}

	var state model.TimerState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return nil, fmt.Errorf("parse timer state: %w", err)
	}
	return &state, nil
}

// TimerStats returns weekly reading stats for the given number of weeks back from now.
func (a *App) TimerStats(weeks int) (model.WeeklyStats, error) {
	if weeks <= 0 {
		weeks = 1
	}
	now := time.Now()

	// Find the start of this week (Monday).
	weekday := (int(now.Weekday()) + 6) % 7 // Mon=0 .. Sun=6
	startOfWeek := time.Date(now.Year(), now.Month(), now.Day()-weekday, 0, 0, 0, 0, now.Location())
	from := startOfWeek.AddDate(0, 0, -(weeks-1)*7)

	return a.Store.GetWeeklyStats(from, now)
}

// TimerList returns the most recent reading sessions.
func (a *App) TimerList(limit int) ([]model.ReadingSession, error) {
	return a.Store.ListSessions(limit)
}

// GetStreak computes the current, longest, and total reading streak from session data.
func (a *App) GetStreak() (*model.StreakInfo, error) {
	// Get all daily activity from the beginning of time.
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Now().UTC()
	activities, err := a.Store.GetDailyActivity(from, to)
	if err != nil {
		return nil, err
	}

	if len(activities) == 0 {
		return &model.StreakInfo{}, nil
	}

	// Build a set of reading days.
	readingDays := make(map[string]bool, len(activities))
	for _, a := range activities {
		if a.Minutes > 0 {
			readingDays[a.Date.Format("2006-01-02")] = true
		}
	}

	today := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.Local)
	yesterday := today.AddDate(0, 0, -1)
	readToday := readingDays[today.Format("2006-01-02")]

	// Longest streak: sort days and find max consecutive run. Computed before
	// the current-streak early return so a broken streak keeps the record.
	days := make([]time.Time, 0, len(readingDays))
	for dateStr := range readingDays {
		if t, err := time.Parse("2006-01-02", dateStr); err == nil {
			days = append(days, t)
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })

	longest := 1
	streak := 1
	for i := 1; i < len(days); i++ {
		diff := days[i].Sub(days[i-1]).Hours() / 24
		if diff <= 1.5 { // consecutive day (accounting for DST)
			streak++
			if streak > longest {
				longest = streak
			}
		} else {
			streak = 1
		}
	}

	// Current streak: count back from today (or yesterday if not read today).
	current := 0
	start := today
	if !readToday {
		if !readingDays[yesterday.Format("2006-01-02")] {
			// No reading today or yesterday, current streak is 0.
			return &model.StreakInfo{
				Longest:   longest,
				Total:     len(readingDays),
				ReadToday: false,
			}, nil
		}
		start = yesterday
	}
	for d := start; ; d = d.AddDate(0, 0, -1) {
		if readingDays[d.Format("2006-01-02")] {
			current++
		} else {
			break
		}
	}

	return &model.StreakInfo{
		Current:   current,
		Longest:   longest,
		Total:     len(readingDays),
		ReadToday: readToday,
	}, nil
}

// GetHeatmap returns daily activity data for the given number of weeks.
func (a *App) GetHeatmap(weeks int) ([]model.DayActivity, error) {
	if weeks <= 0 {
		weeks = 26
	}
	now := time.Now()
	from := now.AddDate(0, 0, -weeks*7)
	return a.Store.GetDailyActivity(from, now)
}
