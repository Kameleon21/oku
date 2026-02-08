package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

const cacheStaleAfter = 6 * time.Hour

// ListBooks returns user books for the given status.
// Serves from cache by default; if refresh is true, fetches from API first.
func (a *App) ListBooks(ctx context.Context, status model.Status, refresh bool) ([]model.UserBook, error) {
	if refresh {
		if err := a.syncStatus(ctx, status); err != nil {
			return nil, err
		}
	}

	books, err := a.Store.ListUserBooks(status)
	if err != nil {
		return nil, err
	}

	// Refresh when cache is empty or stale.
	if !refresh {
		stale, err := a.isStatusCacheStale(status)
		if err != nil {
			return nil, err
		}
		if len(books) == 0 || stale {
			if err := a.syncStatus(ctx, status); err != nil {
				return nil, err
			}
			books, err = a.Store.ListUserBooks(status)
		}
	}

	return books, err
}

func (a *App) isStatusCacheStale(status model.Status) (bool, error) {
	val, err := a.Store.GetState(syncStateKey(status))
	if err != nil {
		return true, err
	}
	if val == "" {
		return true, nil
	}
	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return true, nil
	}
	return time.Since(t) > cacheStaleAfter, nil
}

func syncStateKey(status model.Status) string {
	return fmt.Sprintf("last_sync_status_%d", int(status))
}

// syncStatus fetches books for a status from the API and replaces cached rows for that status.
func (a *App) syncStatus(ctx context.Context, status model.Status) error {
	apiBooks, err := a.API.ListUserBooks(ctx, int(status))
	if err != nil {
		return err
	}

	// Replace rows to avoid stale entries lingering in cache.
	if err := a.Store.DeleteUserBooksByStatus(status); err != nil {
		return err
	}

	for _, ab := range apiBooks {
		ub := convertAPIUserBook(ab)
		if err := a.Store.UpsertUserBook(ub); err != nil {
			return err
		}
		for _, r := range ub.UserBookReads {
			if err := a.Store.UpsertUserBookRead(r); err != nil {
				return err
			}
		}
	}

	_ = a.Store.SetState(syncStateKey(status), time.Now().UTC().Format(time.RFC3339))
	_ = a.Store.PruneOrphanBooks(30)
	return nil
}

func convertAPIUserBook(ab apiUserBookAlias) model.UserBook {
	book := model.Book{
		ID:    ab.Book.ID,
		Title: ab.Book.Title,
		Pages: ab.Book.Pages,
		Slug:  ab.Book.Slug,
	}
	if ab.Book.Image != nil {
		book.ImageURL = ab.Book.Image.URL
	}
	for _, c := range ab.Book.Contributions {
		book.Authors = append(book.Authors, c.Author.Name)
	}

	ub := model.UserBook{
		ID:        ab.ID,
		BookID:    ab.Book.ID,
		StatusID:  model.Status(ab.StatusID),
		Book:      book,
		UpdatedAt: time.Now(),
	}

	for _, r := range ab.UserBookReads {
		read := model.UserBookRead{
			ID:            r.ID,
			UserBookID:    ab.ID,
			ProgressPages: r.ProgressPages,
		}
		ub.UserBookReads = append(ub.UserBookReads, read)
	}

	return ub
}
