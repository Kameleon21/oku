package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/model"
)

func (a *App) AddJournalEntry(ctx context.Context, bookID int, event, text string) (int, error) {
	if err := a.requireOnline(); err != nil {
		return 0, err
	}
	id, err := a.ResolveBookID(bookID)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("text cannot be empty")
	}
	// Unlike progress telemetry, personal notes must never fall back to public.
	privacy, err := a.privacy(ctx, true)
	if err != nil {
		return 0, fmt.Errorf("cannot determine journal privacy: %w", err)
	}
	return a.hc.CreateJournalEntry(ctx, id, event, text, privacy)
}

// BookJournal returns the user's notes and quotes on a book.
func (a *App) BookJournal(ctx context.Context, bookID int) ([]model.BookJournalEntry, error) {
	if err := a.requireOnline(); err != nil {
		return nil, err
	}
	entries, err := a.hc.BookJournal(ctx, bookID)
	if err != nil {
		return nil, err
	}
	return mapBookJournal(entries), nil
}

// ListGoals returns the user's reading goals straight from Hardcover.
func (a *App) ListGoals(ctx context.Context) ([]model.Goal, error) {
	if err := a.requireOnline(); err != nil {
		return nil, err
	}
	apiGoals, err := a.hc.ListGoals(ctx)
	if err != nil {
		return nil, err
	}
	goals := make([]model.Goal, 0, len(apiGoals))
	for _, g := range apiGoals {
		goals = append(goals, mapGoal(g))
	}
	return goals, nil
}

// SaveGoal creates g when id is 0, using the account's privacy setting.
// Otherwise it updates goal id with merge(current, g), so the caller decides
// which of g's fields overwrite the goal's current settings; a nil merge saves
// g as given. The goal is validated as it will be saved.
func (a *App) SaveGoal(ctx context.Context, id int, g model.GoalInput, merge func(old, next model.GoalInput) model.GoalInput) (int, error) {
	if err := a.requireOnline(); err != nil {
		return 0, err
	}
	if id > 0 {
		old, err := a.hc.GoalSettings(ctx, id)
		if err != nil {
			return 0, err
		}
		if merge != nil {
			g = merge(mapGoalInput(old), g)
		}
	} else {
		privacy, err := a.privacy(ctx, true)
		if err != nil {
			return 0, err
		}
		g.PrivacySettingID = privacy
	}
	if err := g.Validate(); err != nil {
		return 0, err
	}
	return a.hc.SaveGoal(ctx, id, apiGoalInput(g))
}

// GetBookDetail returns a book's community metadata, from the cache unless
// refresh is set or nothing is cached. The cache keeps Hardcover's response
// as received, so it is mapped the same way whichever path it comes from.
func (a *App) GetBookDetail(ctx context.Context, id int, refresh bool) (*model.BookDetail, error) {
	key := fmt.Sprintf("book_detail_%d", id)
	if !refresh {
		raw, err := a.store.GetState(key)
		if err != nil {
			return nil, err
		}
		if raw != "" {
			var b api.BookDetail
			if json.Unmarshal([]byte(raw), &b) == nil {
				d := mapBookDetail(b)
				return &d, nil
			}
		}
	}
	if err := a.requireOnline(); err != nil {
		return nil, err
	}
	b, err := a.hc.BookDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	if err := a.store.SetState(key, string(raw)); err != nil {
		return nil, err
	}
	d := mapBookDetail(*b)
	return &d, nil
}

// trendingFetchLimit is the API maximum. Curation drops many of the raw
// results, so fetch the full list and trim afterwards.
const trendingFetchLimit = 100

// TrendingBooks returns Hardcover's trending list. Unless raw is set, books
// already in the library are dropped and each series is represented only by
// its highest-ranked remaining book, because the raw list is usually
// dominated by every volume of one or two popular series.
func (a *App) TrendingBooks(ctx context.Context, period string, limit int, raw bool) ([]model.SearchResult, error) {
	if limit < 1 || limit > trendingFetchLimit {
		return nil, fmt.Errorf("limit must be 1-%d", trendingFetchLimit)
	}
	if err := a.requireOnline(); err != nil {
		return nil, err
	}
	fetch := trendingFetchLimit
	if raw {
		fetch = limit
	}
	books, err := a.hc.Trending(ctx, period, fetch)
	if err != nil {
		return nil, err
	}
	if !raw {
		owned, err := a.store.LibraryBookIDs()
		if err != nil {
			return nil, err
		}
		books = curateTrending(books, owned, limit)
	}
	out := make([]model.SearchResult, 0, len(books))
	for _, b := range books {
		out = append(out, mapTrendingBook(b))
	}
	return out, nil
}

const queueStateKey = "reading_queue_order"

func (a *App) CachedQueueOrder() ([]int, error) {
	raw, err := a.store.GetState(queueStateKey)
	if err != nil {
		return nil, err
	}
	ids := []int{}
	if raw != "" {
		err = json.Unmarshal([]byte(raw), &ids)
	}
	return ids, err
}
func (a *App) RefreshQueue(ctx context.Context) ([]int, error) {
	if err := a.requireOnline(); err != nil {
		return nil, err
	}
	_, rows, err := a.hc.ReadingQueue(ctx, false)
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.BookID)
	}
	return ids, a.saveQueue(ids)
}
func (a *App) saveQueue(ids []int) error {
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return a.store.SetState(queueStateKey, string(raw))
}
func SortQueue(books []model.UserBook, ids []int) {
	rank := map[int]int{}
	for i, id := range ids {
		rank[id] = i
	}
	sort.SliceStable(books, func(i, j int) bool {
		a, oka := rank[books[i].BookID]
		b, okb := rank[books[j].BookID]
		if oka != okb {
			return oka
		}
		return oka && a < b
	})
}

// MoveQueueBook updates one adjacent pair. Missing shelf books are appended to
// the private ranked list; reading status remains independent of priority.
// Like any write-through mutation, a failure to cache the new order is
// ErrCacheRefresh, returned with the order Hardcover now holds.
func (a *App) MoveQueueBook(ctx context.Context, bookID, delta int) ([]int, error) {
	if delta != -1 && delta != 1 {
		return nil, fmt.Errorf("queue direction must be -1 or 1")
	}
	books, err := a.store.ListUserBooks(model.StatusWantToRead)
	if err != nil {
		return nil, err
	}
	found := false
	for _, b := range books {
		if b.BookID == bookID {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("book is not on the want-to-read shelf")
	}
	if err := a.requireOnline(); err != nil {
		return nil, err
	}
	listID, rows, err := a.hc.ReadingQueue(ctx, true)
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	maxPosition := 0
	for _, r := range rows {
		seen[r.BookID] = true
		if r.Position > maxPosition {
			maxPosition = r.Position
		}
	}

	pending := []api.RankedBook{}
	for _, b := range books {
		if !seen[b.BookID] {
			maxPosition++
			pending = append(pending, api.RankedBook{BookID: b.BookID, Position: maxPosition})
		}
	}
	if len(pending) > 0 {
		if err := a.hc.QueueBooks(ctx, listID, pending); err != nil {
			return nil, fmt.Errorf("queue books: %w", err)
		}

		listID, rows, err = a.hc.ReadingQueue(ctx, false)
		if err != nil {
			return nil, err
		}
	}
	wanted := map[int]bool{}
	for _, b := range books {
		wanted[b.BookID] = true
	}
	visible := []int{}
	index := -1
	for i, r := range rows {
		if wanted[r.BookID] {
			if r.BookID == bookID {
				index = len(visible)
			}
			visible = append(visible, i)
		}
	}
	next := index + delta
	if index < 0 {
		return nil, fmt.Errorf("book missing from reading queue")
	}
	if next >= 0 && next < len(visible) {
		i, j := visible[index], visible[next]
		if rows[i].Position == rows[j].Position {
			return nil, fmt.Errorf("queue has duplicate ranks; reorder it on Hardcover and refresh")
		}
		changes := []api.RankedBook{rows[i], rows[j]}
		changes[0].Position, changes[1].Position = changes[1].Position, changes[0].Position
		if err := a.hc.SetQueuePositions(ctx, listID, changes); err != nil {
			return nil, fmt.Errorf("reorder queue: %w", err)
		}
		rows[i], rows[j] = rows[j], rows[i]
	}
	ids := []int{}
	for _, r := range rows {
		ids = append(ids, r.BookID)
	}
	return ids, cacheRefreshed(a.saveQueue(ids))
}

// curateTrending keeps rank order, skips owned books, keeps the first book
// seen for each series and stops at limit.
func curateTrending(books []api.APIBook, owned map[int]bool, limit int) []api.APIBook {
	out := make([]api.APIBook, 0, limit)
	seenSeries := map[int]bool{}
	for _, b := range books {
		if len(out) == limit {
			break
		}
		if owned[b.ID] {
			continue
		}
		if b.FeaturedSeries != nil {
			id := b.FeaturedSeries.Series.ID
			if seenSeries[id] {
				continue
			}
			seenSeries[id] = true
		}
		out = append(out, b)
	}
	return out
}
