package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// UpdateProgress updates the reading progress for a book.
// If bookID is 0, uses the single active book.
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
	latestRead, err := a.Store.GetLatestRead(ub.ID)
	if err != nil {
		return 0, err
	}
	latestRead = unfinishedRead(latestRead)
	if latestRead != nil {
		currentPage = latestRead.ProgressPages
	}

	newPage := pageUpdate.Resolve(currentPage, ub.Book.Pages)

	// Update an existing read entry when available, otherwise create one.
	if latestRead != nil && latestRead.ID > 0 {
		if err := a.API.UpdateReadProgress(ctx, latestRead.ID, newPage); err != nil {
			return 0, fmt.Errorf("update progress: %w", err)
		}
		latestRead.ProgressPages = newPage
		if err := a.Store.UpsertUserBookRead(*latestRead); err != nil {
			return 0, err
		}
		a.logLocalJournal(journalEventProgress)
		a.logRemoteProgress(ctx, resolvedID, newPage, ub.Book.Pages)
		return newPage, nil
	}

	read, err := a.API.InsertUserBookRead(ctx, ub.ID, newPage)
	if err != nil {
		return 0, fmt.Errorf("update progress: %w", err)
	}

	newRead := model.UserBookRead{
		ID:            read.ID,
		UserBookID:    ub.ID,
		ProgressPages: read.ProgressPages,
	}
	if read.StartedAt != nil {
		if t, parseErr := time.Parse("2006-01-02", *read.StartedAt); parseErr == nil {
			newRead.StartedAt = &t
		}
	}
	if read.FinishedAt != nil {
		if t, parseErr := time.Parse("2006-01-02", *read.FinishedAt); parseErr == nil {
			newRead.FinishedAt = &t
		}
	}
	if err := a.Store.UpsertUserBookRead(newRead); err != nil {
		return 0, err
	}

	a.logLocalJournal(journalEventProgress)
	a.logRemoteProgress(ctx, resolvedID, newPage, ub.Book.Pages)
	return newPage, nil
}

func unfinishedRead(read *model.UserBookRead) *model.UserBookRead {
	if read != nil && read.FinishedAt == nil {
		return read
	}
	return nil
}
