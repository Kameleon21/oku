package app

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

const genreBreakdownLimit = 6

// GetReadingStats assembles everything the stats view displays from the local
// cache. It never performs a network request; run a sync first for fresh data.
func (a *App) GetReadingStats() (*model.ReadingStats, error) {
	year := time.Now().Year()

	summary, err := a.store.GetYearSummary(year)
	if err != nil {
		return nil, err
	}
	finished, err := a.store.ListFinishedBooks(year)
	if err != nil {
		return nil, err
	}
	months, err := a.store.GetBooksPerMonth(year)
	if err != nil {
		return nil, err
	}
	years, err := a.store.GetBooksPerYear()
	if err != nil {
		return nil, err
	}
	ratings, err := a.store.GetRatingsDistribution()
	if err != nil {
		return nil, err
	}
	genres, err := a.store.GetGenreBreakdown(genreBreakdownLimit)
	if err != nil {
		return nil, err
	}
	goals, err := a.store.ListGoals()
	if err != nil {
		return nil, err
	}
	heatmap, err := a.GetHeatmap(26)
	if err != nil {
		return nil, err
	}
	weekly, err := a.TimerStats(1)
	if err != nil {
		return nil, err
	}

	return &model.ReadingStats{
		Year:     summary,
		Finished: finished,
		Goal:     pickBooksGoal(goals),
		Months:   months,
		Years:    years,
		Ratings:  ratings,
		Genres:   genres,
		Heatmap:  heatmap,
		Weekly:   weekly,
	}, nil
}

// pickBooksGoal returns the active books goal, preferring one whose window
// covers today. Returns nil when there is none.
func pickBooksGoal(goals []model.Goal) *model.Goal {
	today := time.Now()
	var fallback *model.Goal
	for i := range goals {
		g := goals[i]
		if g.State != "active" || g.Metric != "books" {
			continue
		}
		if fallback == nil {
			fallback = &g
		}
		if !g.EndDate.IsZero() && g.EndDate.Before(today) {
			continue
		}
		return &g
	}
	return fallback
}

// SyncStats refreshes the journal and goal caches from the API. Journal
// history is limited to what the heatmap can show plus margin.
func (a *App) SyncStats(ctx context.Context) error {
	if err := a.requireOnline(); err != nil {
		return err
	}
	userID, err := a.cachedUserID(ctx)
	if err != nil {
		return err
	}

	since := time.Now().AddDate(-1, 0, 0)
	apiJournals, err := a.hc.ListReadingJournals(ctx, userID, since)
	if err != nil {
		return err
	}
	entries := make([]model.JournalEntry, 0, len(apiJournals))
	for _, j := range apiJournals {
		if j.ActionAt == nil {
			continue
		}
		t, ok := parseAPITime(*j.ActionAt)
		if !ok {
			continue
		}
		entries = append(entries, model.JournalEntry{ID: j.ID, ActionAt: t, Event: j.Event})
	}
	if err := a.store.ReplaceReadingJournals(entries); err != nil {
		return err
	}

	apiGoals, err := a.hc.ListGoals(ctx)
	if err != nil {
		return err
	}
	goals := make([]model.Goal, 0, len(apiGoals))
	for _, g := range apiGoals {
		goal := model.Goal{
			ID:       g.ID,
			Metric:   g.Metric,
			Target:   g.Goal,
			Progress: float64(g.Progress),
			State:    g.State,
		}
		if g.StartDate != nil {
			if t, ok := parseAPITime(*g.StartDate); ok {
				goal.StartDate = t
			}
		}
		if g.EndDate != nil {
			if t, ok := parseAPITime(*g.EndDate); ok {
				goal.EndDate = t
			}
		}
		goals = append(goals, goal)
	}
	return a.store.ReplaceGoals(goals)
}

// cachedUserID returns the Hardcover user ID from local state, fetching and
// caching it via the API when absent.
func (a *App) cachedUserID(ctx context.Context) (int, error) {
	if val, err := a.store.GetState("user_id"); err == nil {
		if id, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && id > 0 {
			return id, nil
		}
	}
	if err := a.requireOnline(); err != nil {
		return 0, err
	}
	id, _, err := a.hc.GetMe(ctx)
	if err != nil {
		return 0, err
	}
	_ = a.store.SetState("user_id", strconv.Itoa(id))
	return id, nil
}
