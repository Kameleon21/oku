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

// GetHeatmap returns daily activity data for the given number of weeks,
// merging timer minutes with Hardcover reading-journal days (progress updates,
// finished books) so the heatmap matches the hardcover.app one.
func (a *App) GetHeatmap(weeks int) ([]model.DayActivity, error) {
	if weeks <= 0 {
		weeks = 26
	}
	now := time.Now()
	from := now.AddDate(0, 0, -weeks*7)
	acts, err := a.Store.GetDailyActivity(from, now)
	if err != nil {
		return nil, err
	}
	days, err := a.Store.GetJournalDays(from, now)
	if err != nil {
		return nil, err
	}

	byDate := make(map[string]int, len(acts))
	for i, act := range acts {
		byDate[act.Date.Format("2006-01-02")] = i
	}
	for _, d := range days {
		if i, ok := byDate[d.Date.Format("2006-01-02")]; ok {
			acts[i].Entries = d.Count
			acts[i].HasActivity = true
		} else {
			acts = append(acts, model.DayActivity{Date: d.Date, Entries: d.Count, HasActivity: true})
		}
	}
	sort.Slice(acts, func(i, j int) bool { return acts[i].Date.Before(acts[j].Date) })
	return acts, nil
}
