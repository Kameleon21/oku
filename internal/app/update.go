package app

import (
	"context"
	"fmt"

	"github.com/Kameleon21/oku/internal/model"
)

// UpdateProgress updates the reading progress for a book.
// If bookID is 0, uses the single active book. The new page is returned
// alongside an ErrCacheRefresh error too, since Hardcover already holds it.
func (a *App) UpdateProgress(ctx context.Context, bookID int, pageUpdate model.PageUpdate) (int, error) {
	resolvedID, ub, err := a.loadForWrite(bookID)
	if err != nil {
		return 0, err
	}

	// Get current page from latest read.
	currentPage := 0
	latestRead, err := a.store.GetLatestRead(ub.ID)
	if err != nil {
		return 0, err
	}
	latestRead = unfinishedRead(latestRead)
	if latestRead != nil {
		currentPage = latestRead.ProgressPages
	}

	newPage := pageUpdate.Resolve(currentPage, ub.Book.Pages)

	// Update an existing read entry when available, otherwise create one.
	var read model.UserBookRead
	if latestRead != nil && latestRead.ID > 0 {
		if err := a.hc.UpdateReadProgress(ctx, latestRead.ID, newPage); err != nil {
			return 0, fmt.Errorf("update progress: %w", err)
		}
		read = *latestRead
		read.ProgressPages = newPage
	} else {
		inserted, err := a.hc.InsertUserBookRead(ctx, ub.ID, newPage)
		if err != nil {
			return 0, fmt.Errorf("update progress: %w", err)
		}
		read = model.UserBookRead{
			ID:            inserted.ID,
			UserBookID:    ub.ID,
			ProgressPages: inserted.ProgressPages,
		}
		if inserted.StartedAt != nil {
			if t, ok := parseAPITime(*inserted.StartedAt); ok {
				read.StartedAt = &t
			}
		}
		if inserted.FinishedAt != nil {
			if t, ok := parseAPITime(*inserted.FinishedAt); ok {
				read.FinishedAt = &t
			}
		}
	}

	cacheErr := a.store.UpsertUserBookRead(read)
	a.logLocalJournal(journalEventProgress)
	a.logRemoteProgress(ctx, resolvedID, newPage, ub.Book.Pages)
	return newPage, cacheRefreshed(cacheErr)
}

func unfinishedRead(read *model.UserBookRead) *model.UserBookRead {
	if read != nil && read.FinishedAt == nil {
		return read
	}
	return nil
}
