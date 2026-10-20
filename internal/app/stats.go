package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

const genreBreakdownLimit = 6

// GetReadingStats assembles everything the stats view displays from the local
// cache. It never performs a network request; run a sync first for fresh data.
func (a *App) GetReadingStats() (*model.ReadingStats, error) {
	now := time.Now()
	year := now.Year()

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
	reading, err := a.CachedBooks(model.StatusCurrentlyReading)
	if err != nil {
		return nil, err
	}

	goal := pickBooksGoal(goals)
	var goalPace *model.GoalPace
	if goal != nil {
		if p, ok := model.ForecastGoal(*goal, now); ok {
			goalPace = &p
		}
	}

	return &model.ReadingStats{
		Year:     summary,
		Finished: finished,
		Goal:     goal,
		GoalPace: goalPace,
		Reading:  model.ForecastShelf(reading, now),
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
// history is limited to what the heatmap can show plus margin. Each cache is
// refreshed on its own, so a token that may read goals but not the journal
// (or the other way round) still gets the one it can; the errors are joined.
func (a *App) SyncStats(ctx context.Context) error {
	if err := a.requireOnline(); err != nil {
		return err
	}
	return errors.Join(a.syncJournals(ctx), a.syncGoals(ctx))
}

func (a *App) syncJournals(ctx context.Context) error {
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
		if entry, ok := mapJournalActivity(j); ok {
			entries = append(entries, entry)
		}
	}
	return a.store.ReplaceReadingJournals(entries)
}

func (a *App) syncGoals(ctx context.Context) error {
	apiGoals, err := a.hc.ListGoals(ctx)
	if err != nil {
		return err
	}
	goals := make([]model.Goal, 0, len(apiGoals))
	for _, g := range apiGoals {
		goals = append(goals, mapGoal(g))
	}
	return a.store.ReplaceGoals(goals)
}

// cachedUserID returns the Hardcover user ID from local state, fetching and
// caching it via the API when absent.
func (a *App) cachedUserID(ctx context.Context) (int, error) {
	if val, err := a.store.GetState(userIDKey); err == nil {
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
	if _, err := a.RememberUser(id); err != nil {
		return 0, err
	}
	return id, nil
}
