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

	// A full sync is where a change of account is noticed, so the previous
	// account's books, stamps and queue do not outlive it.
	id, _, err := a.hc.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("sync account: %w", err)
	}
	if err := a.RememberUser(id); err != nil {
		return err
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
