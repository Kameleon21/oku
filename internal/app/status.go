package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// ChangeStatus changes a book's status. If bookID is 0, uses the single active book.
// If the book has no user_book entry yet, creates one via InsertUserBook.
func (a *App) ChangeStatus(ctx context.Context, bookID int, status model.Status) error {
	resolvedID, err := a.ResolveBookID(bookID)
	if err != nil {
		return err
	}

	ub, err := a.Store.GetUserBookByBookID(resolvedID)
	if err != nil {
		return err
	}

	if ub == nil {
		// Book not in user's library yet — insert it.
		_, err := a.API.InsertUserBook(ctx, resolvedID, int(status))
		if err != nil {
			return fmt.Errorf("add book to library: %w", err)
		}
		// The remote status change is the real event, even if a later cache
		// step fails.
		if statusCountsAsActivity(status) {
			a.logLocalJournal(journalEventFinished)
		}
		// Refresh status cache so we get full book metadata from API.
		// Best-effort: the insert already happened remotely, so reporting a
		// cache failure here would make a retry add the book twice.
		_ = a.syncStatus(ctx, status)
		return a.updateActiveBooksForStatus(resolvedID, status)
	}

	// Update existing user_book status.
	if err := a.API.UpdateUserBookStatus(ctx, ub.ID, int(status)); err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if statusCountsAsActivity(status) {
		a.logLocalJournal(journalEventFinished)
	}

	// Update cache. Touch updated_at so the book sorts as recently changed,
	// matching what the API now holds.
	ub.StatusID = status
	ub.UpdatedAt = time.Now().UTC()
	if err := a.Store.UpsertUserBook(*ub); err != nil {
		return err
	}
	return a.updateActiveBooksForStatus(resolvedID, status)
}

func (a *App) updateActiveBooksForStatus(bookID int, status model.Status) error {
	if status == model.StatusCurrentlyReading {
		return a.AddActiveBook(bookID)
	}
	return a.RemoveActiveBook(bookID)
}
