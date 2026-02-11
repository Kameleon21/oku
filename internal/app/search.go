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
	return a.API.SearchBooks(ctx, query, limit, mode)
}
