package tui

import (
	"context"
	"errors"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
)

// backend is what the dashboard needs from the app, in the dashboard's own
// terms: every command in cmds.go is written against it and nothing else.
// *app.App is the production backend; tests substitute an in-memory fake.
type backend interface {
	// Online reports whether Hardcover can be reached. A request that needs
	// it is refused up front when it cannot (see Model.requireOnline); a
	// remote method called regardless answers with app.ErrOffline.
	Online() bool

	// The library: the cached shelves the dashboard opens on, the fresh ones
	// a reload brings in, and the sync that refreshes everything.
	ListCachedBooks(status model.Status) ([]model.UserBook, bool, error)
	CachedBooks(status model.Status) ([]model.UserBook, error)
	ListBooks(ctx context.Context, status model.Status, refresh bool) ([]model.UserBook, error)
	ListAllCachedUserBooks() ([]model.UserBook, error)
	CachedBook(id int) (*model.Book, error)
	CachedQueueOrder() ([]int, error)
	LastSyncAt() time.Time
	SyncAll(ctx context.Context) error

	// Writes that go through to Hardcover. A write that reached Hardcover
	// but not the local cache reports app.ErrCacheRefresh (see writeDone).
	ChangeStatus(ctx context.Context, bookID int, status model.Status) error
	UpdateProgress(ctx context.Context, bookID int, update model.PageUpdate) (int, error)
	ReviewBook(ctx context.Context, bookID int, rating float64, review string) error
	AddJournalEntry(ctx context.Context, bookID int, event, text string) (int, error)
	MoveQueueBook(ctx context.Context, bookID, delta int) ([]int, error)
	RefreshQueue(ctx context.Context) ([]int, error)

	// Reads from Hardcover.
	SearchBooks(ctx context.Context, query string, limit int, mode model.SearchMode) ([]model.SearchResult, error)
	TrendingBooks(ctx context.Context, period string, limit int, raw bool) ([]model.SearchResult, error)
	GetBookDetail(ctx context.Context, id int, refresh bool) (*api.BookDetail, error)
	BookJournal(ctx context.Context, bookID int) ([]api.JournalEntry, error)

	// The local store only: stats, private notes, the search history and
	// the reading timer.
	GetReadingStats() (*model.ReadingStats, error)
	ListPrivateNotes() (map[int]string, error)
	SavePrivateNote(bookID int, text string) error
	RecentSearches() (string, error)
	SaveRecentSearches(raw string) error
	TimerStart(bookID int) error
	TimerStop() (*model.ReadingSession, error)
	TimerStatus() (*model.TimerState, error)
	TimerList(limit int) ([]model.ReadingSession, error)
}

var _ backend = (*app.App)(nil)

// errNoBackend is what the local reads report on a dashboard built without
// an app.
var errNoBackend = errors.New("app not initialized")

// noBackend stands in for a nil app: offline, with no store behind it, so
// every remote call reports app.ErrOffline and every local one errNoBackend.
// It keeps the nil checks out of the commands.
type noBackend struct{}

func (noBackend) Online() bool { return false }

func (noBackend) ListCachedBooks(model.Status) ([]model.UserBook, bool, error) {
	return nil, false, errNoBackend
}
func (noBackend) CachedBooks(model.Status) ([]model.UserBook, error) { return nil, errNoBackend }
func (noBackend) ListBooks(context.Context, model.Status, bool) ([]model.UserBook, error) {
	return nil, errNoBackend
}
func (noBackend) ListAllCachedUserBooks() ([]model.UserBook, error) { return nil, errNoBackend }
func (noBackend) CachedBook(int) (*model.Book, error)               { return nil, errNoBackend }
func (noBackend) CachedQueueOrder() ([]int, error)                  { return nil, errNoBackend }
func (noBackend) LastSyncAt() time.Time                             { return time.Time{} }
func (noBackend) SyncAll(context.Context) error                     { return app.ErrOffline }

func (noBackend) ChangeStatus(context.Context, int, model.Status) error { return app.ErrOffline }
func (noBackend) UpdateProgress(context.Context, int, model.PageUpdate) (int, error) {
	return 0, app.ErrOffline
}
func (noBackend) ReviewBook(context.Context, int, float64, string) error { return app.ErrOffline }
func (noBackend) AddJournalEntry(context.Context, int, string, string) (int, error) {
	return 0, app.ErrOffline
}
func (noBackend) MoveQueueBook(context.Context, int, int) ([]int, error) { return nil, app.ErrOffline }
func (noBackend) RefreshQueue(context.Context) ([]int, error)            { return nil, app.ErrOffline }

func (noBackend) SearchBooks(context.Context, string, int, model.SearchMode) ([]model.SearchResult, error) {
	return nil, app.ErrOffline
}
func (noBackend) TrendingBooks(context.Context, string, int, bool) ([]model.SearchResult, error) {
	return nil, app.ErrOffline
}
func (noBackend) GetBookDetail(context.Context, int, bool) (*api.BookDetail, error) {
	return nil, app.ErrOffline
}
func (noBackend) BookJournal(context.Context, int) ([]api.JournalEntry, error) {
	return nil, app.ErrOffline
}

func (noBackend) GetReadingStats() (*model.ReadingStats, error) { return nil, errNoBackend }
func (noBackend) ListPrivateNotes() (map[int]string, error)     { return nil, errNoBackend }
func (noBackend) SavePrivateNote(int, string) error             { return errNoBackend }
func (noBackend) RecentSearches() (string, error)               { return "", errNoBackend }
func (noBackend) SaveRecentSearches(string) error               { return errNoBackend }
func (noBackend) TimerStart(int) error                          { return errNoBackend }
func (noBackend) TimerStop() (*model.ReadingSession, error)     { return nil, errNoBackend }
func (noBackend) TimerStatus() (*model.TimerState, error)       { return nil, errNoBackend }
func (noBackend) TimerList(int) ([]model.ReadingSession, error) { return nil, errNoBackend }
