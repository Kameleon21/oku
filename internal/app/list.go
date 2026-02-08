package app

import (
	"context"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

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

	// If cache is empty and we haven't refreshed yet, try fetching.
	if len(books) == 0 && !refresh {
		if err := a.syncStatus(ctx, status); err != nil {
			return nil, err
		}
		books, err = a.Store.ListUserBooks(status)
	}

	return books, err
}

// syncStatus fetches books for a status from the API and caches them.
func (a *App) syncStatus(ctx context.Context, status model.Status) error {
	apiBooks, err := a.API.ListUserBooks(ctx, int(status))
	if err != nil {
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
