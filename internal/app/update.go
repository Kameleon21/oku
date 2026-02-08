package app

import (
	"context"
	"fmt"

	"github.com/Kameleon21/oku/internal/model"
)

// UpdateProgress updates the reading progress for a book.
// If bookID is 0, uses the active book.
func (a *App) UpdateProgress(ctx context.Context, bookID int, pageUpdate model.PageUpdate) (int, error) {
	resolvedID, err := a.ResolveBookID(bookID)
	if err != nil {
		return 0, err
	}

	ub, err := a.Store.GetUserBookByBookID(resolvedID)
	if err != nil {
		return 0, err
	}
	if ub == nil {
		return 0, fmt.Errorf("book ID %d not found in cache. Run: oku sync", resolvedID)
	}

	// Get current page from latest read.
	currentPage := 0
	latestRead, _ := a.Store.GetLatestRead(ub.ID)
	if latestRead != nil {
		currentPage = latestRead.ProgressPages
	}

	newPage := pageUpdate.Resolve(currentPage, ub.Book.Pages)

	// Update via API using upsert.
	if err := a.API.UpsertUserBookReads(ctx, ub.ID, newPage); err != nil {
		return 0, fmt.Errorf("update progress: %w", err)
	}

	// Update local cache.
	if latestRead != nil {
		latestRead.ProgressPages = newPage
		_ = a.Store.UpsertUserBookRead(*latestRead)
	}

	return newPage, nil
}
