package app

import (
	"context"
	"fmt"

	"github.com/Kameleon21/oku/internal/model"
)

// SyncAll refreshes all tracked status lists into the local cache.
func (a *App) SyncAll(ctx context.Context) error {
	statuses := []model.Status{
		model.StatusWantToRead,
		model.StatusCurrentlyReading,
		model.StatusRead,
		model.StatusDidNotFinish,
	}

	for _, s := range statuses {
		if err := a.syncStatus(ctx, s); err != nil {
			return fmt.Errorf("sync %s: %w", s, err)
		}
	}

	return nil
}
