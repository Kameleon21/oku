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
	id, err := a.ResolveBookID(bookID)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(text) == "" {
		return 0, fmt.Errorf("text cannot be empty")
	}
	// Unlike progress telemetry, personal notes must never fall back to public.
	privacy, err := a.API.GetAccountPrivacySetting(ctx)
	if err != nil {
		return 0, fmt.Errorf("cannot determine journal privacy: %w", err)
	}
	return a.API.CreateJournalEntry(ctx, id, event, text, privacy)
}
func (a *App) GetBookDetail(ctx context.Context, id int, refresh bool) (*api.BookDetail, error) {
	key := fmt.Sprintf("book_detail_%d", id)
	if !refresh {
		raw, err := a.Store.GetState(key)
		if err != nil {
			return nil, err
		}
		if raw != "" {
			var b api.BookDetail
			if json.Unmarshal([]byte(raw), &b) == nil {
				return &b, nil
			}
		}
	}
	b, err := a.API.BookDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	if err := a.Store.SetState(key, string(raw)); err != nil {
		return nil, err
	}
	return b, nil
}
func (a *App) TrendingBooks(ctx context.Context, period string, limit int) ([]model.SearchResult, error) {
	books, err := a.API.Trending(ctx, period, limit)
	if err != nil {
		return nil, err
	}
	out := make([]model.SearchResult, 0, len(books))
	for _, b := range books {
		r := model.SearchResult{ID: b.ID, Title: b.Title, Pages: b.Pages, Slug: b.Slug, Rating: b.Rating, Ratings: b.RatingsCount}
		for _, c := range b.Contributions {
			r.Authors = append(r.Authors, c.Author.Name)
		}
		out = append(out, r)
	}
	return out, nil
}

const queueStateKey = "reading_queue_order"

func (a *App) CachedQueueOrder() ([]int, error) {
	raw, err := a.Store.GetState(queueStateKey)
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
	_, rows, err := a.API.ReadingQueue(ctx, false)
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
	return a.Store.SetState(queueStateKey, string(raw))
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
func (a *App) MoveQueueBook(ctx context.Context, bookID, delta int) ([]int, error) {
	if delta != -1 && delta != 1 {
		return nil, fmt.Errorf("queue direction must be -1 or 1")
	}
	books, err := a.Store.ListUserBooks(model.StatusWantToRead)
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
	listID, rows, err := a.API.ReadingQueue(ctx, true)
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
		if err := a.API.QueueBooks(ctx, listID, pending); err != nil {
			return nil, err
		}

		listID, rows, err = a.API.ReadingQueue(ctx, false)
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
		if err := a.API.SetQueuePositions(ctx, listID, changes); err != nil {
			return nil, err
		}
		rows[i], rows[j] = rows[j], rows[i]
	}
	ids := []int{}
	for _, r := range rows {
		ids = append(ids, r.BookID)
	}
	if err := a.saveQueue(ids); err != nil {
		return ids, fmt.Errorf("ranking saved remotely; local cache failed: %w", err)
	}
	return ids, nil
}
