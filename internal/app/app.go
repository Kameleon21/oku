package app

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/config"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/store"
)

// App is the core engine that orchestrates the API client and local store.
type App struct {
	API    *api.Client
	Store  *store.Store
	Config config.Config
}

// New creates a new App instance.
func New(apiClient *api.Client, db *store.Store, cfg config.Config) *App {
	return &App{
		API:    apiClient,
		Store:  db,
		Config: cfg,
	}
}

// ValidateToken calls GetMe to verify the token works and caches the user ID.
func (a *App) ValidateToken(ctx context.Context) (int, string, error) {
	id, username, err := a.API.GetMe(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("token validation failed: %w", err)
	}
	_ = a.Store.SetState("user_id", strconv.Itoa(id))
	_ = a.Store.SetState("username", username)
	return id, username, nil
}

// GetActiveBookID returns the active book ID from local state.
// If no active book is explicitly set, it falls back to the user's
// currently-reading books from the cache — auto-selecting if there's exactly one.
func (a *App) GetActiveBookID() (int, error) {
	val, err := a.Store.GetState("active_book_id")
	if err != nil {
		return 0, err
	}
	if val != "" {
		id, err := strconv.Atoi(val)
		if err != nil {
			return 0, fmt.Errorf("invalid active book ID: %s", val)
		}
		return id, nil
	}

	// No explicit active book — try to auto-detect from currently reading.
	books, err := a.Store.ListUserBooks(model.StatusCurrentlyReading)
	if err != nil {
		return 0, err
	}
	if len(books) == 1 {
		// Auto-set when there's exactly one currently-reading book.
		_ = a.SetActiveBook(books[0].BookID)
		return books[0].BookID, nil
	}
	if len(books) > 1 {
		return 0, fmt.Errorf("multiple books currently reading. Use: oku open")
	}
	return 0, fmt.Errorf("no active book set. Use: oku set-active --book <id> or oku open")
}

// SetActiveBook sets the active book by book ID.
func (a *App) SetActiveBook(bookID int) error {
	return a.Store.SetState("active_book_id", strconv.Itoa(bookID))
}

// GetActiveBook returns the active UserBook from cache.
func (a *App) GetActiveBook() (*model.UserBook, error) {
	bookID, err := a.GetActiveBookID()
	if err != nil {
		return nil, err
	}
	ub, err := a.Store.GetUserBookByBookID(bookID)
	if err != nil {
		return nil, err
	}
	if ub == nil {
		return nil, fmt.Errorf("active book (ID %d) not found in cache. Run: oku sync", bookID)
	}
	return ub, nil
}

// ResolveBookID returns the given bookID if > 0, otherwise the active book ID.
func (a *App) ResolveBookID(bookID int) (int, error) {
	if bookID > 0 {
		return bookID, nil
	}
	return a.GetActiveBookID()
}
