package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/config"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/store"
)

const (
	// Legacy key kept for migration from older versions.
	activeBookIDKey  = "active_book_id"
	activeBookIDsKey = "active_book_ids"
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

// GetActiveBookIDs returns the list of active reading book IDs.
func (a *App) GetActiveBookIDs() ([]int, error) {
	ids, err := a.getStoredActiveBookIDs()
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		return ids, nil
	}

	// Migrate legacy single active ID if present.
	val, err := a.Store.GetState(activeBookIDKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(val) != "" {
		id, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("invalid active book ID: %s", val)
		}
		ids = []int{id}
		_ = a.saveActiveBookIDs(ids)
		return ids, nil
	}

	// Bootstrap from currently reading books in cache.
	books, err := a.Store.ListUserBooks(model.StatusCurrentlyReading)
	if err != nil {
		return nil, err
	}
	ids = make([]int, 0, len(books))
	for _, b := range books {
		ids = append(ids, b.BookID)
	}
	ids = sanitizeActiveIDs(ids)
	if len(ids) > 0 {
		_ = a.saveActiveBookIDs(ids)
	}
	return ids, nil
}

// GetActiveBookID returns the active book ID from local state.
// When multiple active books exist, callers must pass --book explicitly.
func (a *App) GetActiveBookID() (int, error) {
	ids, err := a.GetActiveBookIDs()
	if err != nil {
		return 0, err
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	if len(ids) > 1 {
		return 0, fmt.Errorf("multiple active books. Use --book <id>")
	}
	return 0, fmt.Errorf("no active books set. Move a book to reading first")
}

// GetActiveBooks returns all active books found in cache.
func (a *App) GetActiveBooks() ([]model.UserBook, error) {
	ids, err := a.GetActiveBookIDs()
	if err != nil {
		return nil, err
	}
	books := make([]model.UserBook, 0, len(ids))
	validIDs := make([]int, 0, len(ids))

	for _, id := range ids {
		ub, err := a.Store.GetUserBookByBookID(id)
		if err != nil {
			return nil, err
		}
		if ub == nil || ub.StatusID != model.StatusCurrentlyReading {
			continue
		}
		books = append(books, *ub)
		validIDs = append(validIDs, id)
	}

	// Keep state clean when entries are no longer valid.
	if len(validIDs) != len(ids) {
		_ = a.saveActiveBookIDs(validIDs)
	}

	return books, nil
}

// AddActiveBook appends a book to the active list if missing.
func (a *App) AddActiveBook(bookID int) error {
	if bookID <= 0 {
		return fmt.Errorf("invalid book ID: %d", bookID)
	}
	ids, err := a.GetActiveBookIDs()
	if err != nil {
		return err
	}
	ids = append(ids, bookID)
	return a.saveActiveBookIDs(ids)
}

// RemoveActiveBook removes a book from the active list.
func (a *App) RemoveActiveBook(bookID int) error {
	if bookID <= 0 {
		return nil
	}
	ids, err := a.GetActiveBookIDs()
	if err != nil {
		return err
	}
	filtered := make([]int, 0, len(ids))
	for _, id := range ids {
		if id != bookID {
			filtered = append(filtered, id)
		}
	}
	if err := a.saveActiveBookIDs(filtered); err != nil {
		return err
	}
	return nil
}

// SetActiveBook adds a book to the active list by book ID.
func (a *App) SetActiveBook(bookID int) error {
	return a.AddActiveBook(bookID)
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

func (a *App) getStoredActiveBookIDs() ([]int, error) {
	val, err := a.Store.GetState(activeBookIDsKey)
	if err != nil {
		return nil, err
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return nil, nil
	}

	var ids []int
	if err := json.Unmarshal([]byte(val), &ids); err == nil {
		return sanitizeActiveIDs(ids), nil
	}

	parts := strings.Split(val, ",")
	ids = make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid active book IDs: %s", val)
		}
		ids = append(ids, id)
	}
	return sanitizeActiveIDs(ids), nil
}

func (a *App) saveActiveBookIDs(ids []int) error {
	ids = sanitizeActiveIDs(ids)
	if len(ids) == 0 {
		return a.Store.SetState(activeBookIDsKey, "")
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return a.Store.SetState(activeBookIDsKey, string(raw))
}

func sanitizeActiveIDs(ids []int) []int {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
