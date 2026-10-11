package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

const (
	cacheStaleAfter = 6 * time.Hour

	// orphanBookMaxAgeDays is how long a cached book with no user_books row is
	// kept before pruning.
	orphanBookMaxAgeDays = 30
)

// ListBooks returns user books for the given status.
// Serves from cache by default; if refresh is true, fetches from API first.
func (a *App) ListBooks(ctx context.Context, status model.Status, refresh bool) ([]model.UserBook, error) {
	if refresh {
		if err := a.syncStatus(ctx, status); err != nil {
			return nil, err
		}
	}

	books, err := a.store.ListUserBooks(status)
	if err != nil {
		return nil, err
	}

	// Refresh when cache is empty or stale.
	if !refresh {
		var stale bool
		stale, err = a.isStatusCacheStale(status)
		if err != nil {
			return nil, err
		}
		if len(books) == 0 || stale {
			if err := a.syncStatus(ctx, status); err != nil {
				return nil, err
			}
			books, err = a.store.ListUserBooks(status)
			if err != nil {
				return nil, err
			}
		}
	}

	return books, nil
}

// CachedBooks returns the locally cached books for a status without checking
// freshness. It never performs a network request.
func (a *App) CachedBooks(status model.Status) ([]model.UserBook, error) {
	return a.store.ListUserBooks(status)
}

func (a *App) isStatusCacheStale(status model.Status) (bool, error) {
	val, err := a.store.GetState(syncStateKey(status))
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

// LastSyncAt is when the library was last synced with Hardcover: the newest
// of the per-status stamps, or the zero time when it never was.
func (a *App) LastSyncAt() time.Time {
	var latest time.Time
	for _, status := range model.AllStatuses {
		raw, err := a.store.GetState(syncStateKey(status))
		if err != nil || raw == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, raw); err == nil && t.After(latest) {
			latest = t
		}
	}
	return latest
}

// syncStatus refreshes a single status and prunes books no status references
// any more. Multi-status syncs use syncStatusOnly and prune once at the end.
func (a *App) syncStatus(ctx context.Context, status model.Status) error {
	if err := a.syncStatusOnly(ctx, status); err != nil {
		return err
	}
	_ = a.store.PruneOrphanBooks(orphanBookMaxAgeDays)
	return nil
}

// syncStatusOnly fetches books for a status from the API and replaces cached rows for that status.
func (a *App) syncStatusOnly(ctx context.Context, status model.Status) error {
	if err := a.requireOnline(); err != nil {
		return err
	}
	apiBooks, err := a.hc.ListUserBooks(ctx, int(status))
	if err != nil {
		return err
	}

	// Replace rows atomically so a mid-sync failure keeps the previous cache.
	books := make([]model.UserBook, 0, len(apiBooks))
	for _, ab := range apiBooks {
		books = append(books, mapUserBook(ab))
	}
	if err := a.store.ReplaceUserBooksForStatus(status, books); err != nil {
		return err
	}

	_ = a.store.SetState(syncStateKey(status), time.Now().UTC().Format(time.RFC3339))
	return nil
}
