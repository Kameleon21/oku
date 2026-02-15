package app

import (
	"context"

	"github.com/Kameleon21/oku/internal/model"
)

// SearchBooks queries the Hardcover API for books matching the query and mode.
// Search always hits the API (not cache) since it uses Typesense.
func (a *App) SearchBooks(ctx context.Context, query string, limit int, mode model.SearchMode) ([]model.SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	if mode == "" {
		mode = model.SearchModeBook
	}
	results, err := a.API.SearchBooks(ctx, query, limit, mode)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return results, nil
	}

	ids := make([]int, 0, len(results))
	for _, r := range results {
		if r.ID > 0 {
			ids = append(ids, r.ID)
		}
	}

	// Enrich search hits with ratings/engagement metadata when available.
	byID, err := a.API.GetBookRatingsByIDs(ctx, ids)
	if err != nil {
		// Keep search usable even if enrichment query fails.
		return results, nil
	}
	for i := range results {
		meta, ok := byID[results[i].ID]
		if !ok {
			continue
		}
		if results[i].Rating <= 0 && meta.Rating > 0 {
			results[i].Rating = meta.Rating
		}
		if results[i].Ratings <= 0 && meta.RatingsCount > 0 {
			results[i].Ratings = meta.RatingsCount
		}
	}

	return results, nil
}
