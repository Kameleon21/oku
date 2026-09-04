package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// ErrCacheRefresh reports that a write reached Hardcover but the local cache
// could not be refreshed afterwards. The remote change stands, so callers
// should report success and warn rather than fail. The message is user-facing:
// the TUI renders err.Error() verbatim.
var ErrCacheRefresh = errors.New("book updated on Hardcover but the local cache could not be refreshed; run `oku sync`")

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
		syncErr := a.syncStatus(ctx, status)
		if err := a.updateActiveBooksForStatus(resolvedID, status); err != nil {
			return err
		}
		if syncErr != nil {
			// The insert already happened remotely, so this must not read as a
			// plain failure (a retry would add the book twice) nor as a clean
			// success (the book is missing from the cached list).
			return fmt.Errorf("%w: %v", ErrCacheRefresh, syncErr)
		}
		return nil
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
