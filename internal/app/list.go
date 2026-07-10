package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

const cacheStaleAfter = 6 * time.Hour

// ListBooks returns user books for the given status.
// Serves from cache by default; if refresh is true, fetches from API first.
func (a *App) ListBooks(ctx context.Context, status model.Status, refresh bool) ([]model.UserBook, error) {
	if refresh {
		if err := a.syncStatus(ctx, status); err != nil {
			return nil, err
		}
	}

	books, err := a.Store.ListUserBooks(status)
	if err != nil {
		return nil, err
	}

	// Refresh when cache is empty or stale.
	if !refresh {
		stale, err := a.isStatusCacheStale(status)
		if err != nil {
			return nil, err
		}
		if len(books) == 0 || stale {
			if err := a.syncStatus(ctx, status); err != nil {
				return nil, err
			}
			books, err = a.Store.ListUserBooks(status)
		}
	}

	return books, err
}

// ListCachedBooks returns locally cached books and reports whether the cache
// should be refreshed. It never performs a network request.
func (a *App) ListCachedBooks(status model.Status) ([]model.UserBook, bool, error) {
	books, err := a.Store.ListUserBooks(status)
	if err != nil {
		return nil, false, err
	}

	stale, err := a.isStatusCacheStale(status)
	if err != nil {
		return nil, false, err
	}
	return books, len(books) == 0 || stale, nil
}

func (a *App) isStatusCacheStale(status model.Status) (bool, error) {
	val, err := a.Store.GetState(syncStateKey(status))
	if err != nil {
		return true, err
	}
	if val == "" {
		return true, nil
	}
	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return true, nil
	}
	return time.Since(t) > cacheStaleAfter, nil
}

func syncStateKey(status model.Status) string {
	return fmt.Sprintf("last_sync_status_%d", int(status))
}

// syncStatus fetches books for a status from the API and replaces cached rows for that status.
func (a *App) syncStatus(ctx context.Context, status model.Status) error {
	apiBooks, err := a.API.ListUserBooks(ctx, int(status))
	if err != nil {
		return err
	}

	// Replace rows to avoid stale entries lingering in cache.
	if err := a.Store.DeleteUserBooksByStatus(status); err != nil {
		return err
	}

	for _, ab := range apiBooks {
		ub := convertAPIUserBook(ab)
		if err := a.Store.UpsertUserBook(ub); err != nil {
			return err
		}
		for _, r := range ub.UserBookReads {
			if err := a.Store.UpsertUserBookRead(r); err != nil {
				return err
			}
		}
	}

	_ = a.Store.SetState(syncStateKey(status), time.Now().UTC().Format(time.RFC3339))
	_ = a.Store.PruneOrphanBooks(30)
	return nil
}

func convertAPIUserBook(ab apiUserBookAlias) model.UserBook {
	book := model.Book{
		ID:             ab.Book.ID,
		Title:          ab.Book.Title,
		Pages:          ab.Book.Pages,
		Slug:           ab.Book.Slug,
		Rating:         ab.Book.Rating,
		RatingsCount:   ab.Book.RatingsCount,
		ReviewsCount:   ab.Book.ReviewsCount,
		UsersCount:     ab.Book.UsersCount,
		UsersReadCount: ab.Book.UsersReadCount,
	}
	if ab.Book.Image != nil {
		book.ImageURL = ab.Book.Image.URL
	}
	if ab.Book.ReleaseDate != nil {
		book.ReleaseDate = strings.TrimSpace(*ab.Book.ReleaseDate)
	}
	for _, c := range ab.Book.Contributions {
		book.Authors = append(book.Authors, c.Author.Name)
	}

	updatedAt := time.Now().UTC()
	if ab.UpdatedAt != nil {
		if t, ok := parseAPITime(*ab.UpdatedAt); ok {
			updatedAt = t
		}
	}

	ub := model.UserBook{
		ID:        ab.ID,
		BookID:    ab.Book.ID,
		StatusID:  model.Status(ab.StatusID),
		Book:      book,
		UpdatedAt: updatedAt,
	}
	if ab.Rating != nil {
		ub.Rating = *ab.Rating
	}
	if ab.ReviewRaw != nil {
		ub.Review = *ab.ReviewRaw
	}
	if ab.ReviewedAt != nil {
		if t, ok := parseAPITime(*ab.ReviewedAt); ok {
			ub.ReviewedAt = &t
		}
	}

	for _, r := range ab.UserBookReads {
		read := model.UserBookRead{
			ID:            r.ID,
			UserBookID:    ab.ID,
			ProgressPages: r.ProgressPages,
		}
		if r.StartedAt != nil {
			if t, ok := parseAPITime(*r.StartedAt); ok {
				read.StartedAt = &t
			}
		}
		if r.FinishedAt != nil {
			if t, ok := parseAPITime(*r.FinishedAt); ok {
				read.FinishedAt = &t
			}
		}
		ub.UserBookReads = append(ub.UserBookReads, read)
	}

	return ub
}

func parseAPITime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
