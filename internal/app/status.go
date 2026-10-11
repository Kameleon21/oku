package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// ChangeStatus changes a book's status. If bookID is 0, uses the single active book.
// If the book has no user_book entry yet, creates one via InsertUserBook.
func (a *App) ChangeStatus(ctx context.Context, bookID int, status model.Status) error {
	resolvedID, ub, err := a.loadForWrite(bookID)
	if errors.Is(err, errNotCached) {
		return a.addToLibrary(ctx, resolvedID, status)
	}
	if err != nil {
		return err
	}

	if err := a.hc.UpdateUserBookStatus(ctx, ub.ID, int(status)); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if statusCountsAsActivity(status) {
		a.logLocalJournal(journalEventFinished)
	}

	// Update cache. Touch updated_at so the book sorts as recently changed,
	// matching what the API now holds.
	ub.StatusID = status
	ub.UpdatedAt = time.Now().UTC()
	upsertErr := a.store.UpsertUserBook(*ub)
	activeErr := a.updateActiveBooksForStatus(resolvedID, status)
	return cacheRefreshed(cmp.Or(upsertErr, activeErr))
}

// addToLibrary inserts a book that is not in the user's library yet with the
// given status, then syncs that status so the cache holds Hardcover's full
// row.
func (a *App) addToLibrary(ctx context.Context, bookID int, status model.Status) error {
	if _, err := a.hc.InsertUserBook(ctx, bookID, int(status)); err != nil {
		return fmt.Errorf("add book to library: %w", err)
	}
	if statusCountsAsActivity(status) {
		a.logLocalJournal(journalEventFinished)
	}
	syncErr := a.syncStatus(ctx, status)
	activeErr := a.updateActiveBooksForStatus(bookID, status)
	return cacheRefreshed(cmp.Or(syncErr, activeErr))
}

func (a *App) updateActiveBooksForStatus(bookID int, status model.Status) error {
	if status == model.StatusCurrentlyReading {
		return a.AddActiveBook(bookID)
	}
	return a.RemoveActiveBook(bookID)
}
