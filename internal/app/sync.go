package app

import (
	"context"
	"fmt"

	"github.com/Kameleon21/oku/internal/model"
)

// SyncAll refreshes all tracked status lists into the local cache.
func (a *App) SyncAll(ctx context.Context) error {
	if err := a.requireOnline(); err != nil {
		return err
	}

	// A full sync also checks which account is signed in, so a previous
	// account's books, stamps and queue do not outlive it. The check is a
	// backstop to the one at login: when it cannot be made the sync goes on
	// without it, and fails only on the library itself.
	if id, _, err := a.hc.GetMe(ctx); err == nil {
		if _, err := a.RememberUser(id); err != nil {
			return err
		}
	}

	for _, s := range model.AllStatuses {
		if err := a.syncStatusOnly(ctx, s); err != nil {
			return fmt.Errorf("sync %s: %w", s, err)
		}
	}

	// Prune once, after every status has been replaced, so a book that only
	// moved between statuses is never seen as an orphan mid-sync.
	_ = a.store.PruneOrphanBooks(orphanBookMaxAgeDays)

	// Journals and goals feed the stats view only; their failure should not
	// fail the core library sync.
	_ = a.SyncStats(ctx)

	return nil
}
