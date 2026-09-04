package app

import (
	"context"
	"fmt"

	"github.com/Kameleon21/oku/internal/model"
)

// SyncAll refreshes all tracked status lists into the local cache.
func (a *App) SyncAll(ctx context.Context) error {
	for _, s := range model.AllStatuses {
		if err := a.syncStatusOnly(ctx, s); err != nil {
			return fmt.Errorf("sync %s: %w", s, err)
		}
	}

	// Prune once, after every status has been replaced, so a book that only
	// moved between statuses is never seen as an orphan mid-sync.
	_ = a.Store.PruneOrphanBooks(orphanBookMaxAgeDays)

	// Journals and goals feed the stats view only; their failure should not
	// fail the core library sync.
	_ = a.SyncStats(ctx)

	return nil
}
