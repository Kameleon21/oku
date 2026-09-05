package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// RateBook updates a rating for a book in the user's library.
func (a *App) RateBook(ctx context.Context, bookID int, rating float64) error {
	if err := model.ValidateRating(rating); err != nil {
		return err
	}

	resolvedID, err := a.ResolveBookID(bookID)
	if err != nil {
		return err
	}

	ub, err := a.Store.GetUserBookByBookID(resolvedID)
	if err != nil {
		return err
	}
	if ub == nil {
		return fmt.Errorf("book ID %d not found in cache. Run: oku sync", resolvedID)
	}

	if err := a.API.UpdateUserBookRating(ctx, ub.ID, rating); err != nil {
		return fmt.Errorf("update rating: %w", err)
	}

	ub.Rating = rating
	ub.UpdatedAt = time.Now().UTC()
	if err := a.Store.UpsertUserBook(*ub); err != nil {
		return err
	}
	return nil
}

// ReviewBook updates both rating and review text for a book in the user's library.
func (a *App) ReviewBook(ctx context.Context, bookID int, rating float64, review string) error {
	if err := model.ValidateRating(rating); err != nil {
		return err
	}

	resolvedID, err := a.ResolveBookID(bookID)
	if err != nil {
		return err
	}

	ub, err := a.Store.GetUserBookByBookID(resolvedID)
	if err != nil {
		return err
	}
	if ub == nil {
		return fmt.Errorf("book ID %d not found in cache. Run: oku sync", resolvedID)
	}

	reviewedAt := time.Now().UTC()
	if err := a.API.UpdateUserBookReviewAndRating(ctx, ub.ID, rating, review, reviewedAt.Format(time.DateOnly)); err != nil {
		return fmt.Errorf("update review: %w", err)
	}

	ub.Rating = rating
	ub.Review = review
	ub.ReviewedAt = &reviewedAt
	ub.UpdatedAt = reviewedAt
	if err := a.Store.UpsertUserBook(*ub); err != nil {
		return err
	}
	return nil
}

// GetUserBookForTitle returns a single cached user book by title match.
// It prefers exact case-insensitive matches, then substring matches.
// Returns nil when there is no unambiguous match.
func (a *App) GetUserBookForTitle(title string) (*model.UserBook, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, nil
	}

	all, err := a.ListAllCachedUserBooks()
	if err != nil {
		return nil, err
	}

	lower := strings.ToLower(title)
	exact := make([]model.UserBook, 0, 2)
	for _, b := range all {
		if strings.EqualFold(strings.TrimSpace(b.Book.Title), title) {
			exact = append(exact, b)
		}
	}
	if len(exact) == 1 {
		b := exact[0]
		return &b, nil
	}
	if len(exact) > 1 {
		return nil, nil
	}

	partial := make([]model.UserBook, 0, 4)
	for _, b := range all {
		if strings.Contains(strings.ToLower(strings.TrimSpace(b.Book.Title)), lower) {
			partial = append(partial, b)
		}
	}
	if len(partial) == 1 {
		b := partial[0]
		return &b, nil
	}
	return nil, nil
}

// ListAllCachedUserBooks returns every cached user book across all statuses,
// de-duplicated by book ID.
func (a *App) ListAllCachedUserBooks() ([]model.UserBook, error) {
	seen := map[int]struct{}{}
	out := make([]model.UserBook, 0, 64)
	for _, status := range model.AllStatuses {
		books, err := a.Store.ListUserBooks(status)
		if err != nil {
			return nil, err
		}
		for _, b := range books {
			if _, ok := seen[b.Book.ID]; ok {
				continue
			}
			seen[b.Book.ID] = struct{}{}
			out = append(out, b)
		}
	}
	return out, nil
}
