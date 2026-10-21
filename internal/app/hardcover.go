package app

import (
	"context"
	"errors"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/model"
)

// Hardcover is the seam between App and the Hardcover API: every remote call
// App makes goes through it. *api.Client is the production adapter; tests
// substitute an in-memory fake. It speaks api types; hardcover_map.go turns
// them into model types.
type Hardcover interface {
	GetMe(ctx context.Context) (int, string, error)
	GetAccountPrivacySetting(ctx context.Context) (int, error)

	ListUserBooks(ctx context.Context, statusID int) ([]api.APIUserBook, error)
	InsertUserBook(ctx context.Context, bookID int, statusID int) (int, error)
	UpdateUserBookStatus(ctx context.Context, userBookID int, statusID int) error
	UpdateUserBookRating(ctx context.Context, userBookID int, rating float64) error
	UpdateUserBookReviewAndRating(ctx context.Context, userBookID int, rating float64, review string, reviewedAt string) error
	LatestOpenRead(ctx context.Context, userBookID int) (*api.APIUserBookRead, error)
	InsertUserBookRead(ctx context.Context, userBookID int, progressPages int) (*api.APIUserBookRead, error)
	UpdateReadProgress(ctx context.Context, userBookReadID int, progressPages int) error

	InsertProgressJournal(ctx context.Context, bookID, progressPages, totalPages, privacySettingID int) error
	ListReadingJournals(ctx context.Context, userID int, since time.Time) ([]api.APIReadingJournal, error)
	CreateJournalEntry(ctx context.Context, bookID int, event, entry string, privacy int) (int, error)
	BookJournal(ctx context.Context, bookID int) ([]api.JournalEntry, error)

	ListGoals(ctx context.Context) ([]api.APIGoal, error)
	GoalSettings(ctx context.Context, id int) (api.GoalInput, error)
	SaveGoal(ctx context.Context, id int, g api.GoalInput) (int, error)

	SearchBooks(ctx context.Context, query string, perPage int, mode model.SearchMode) ([]api.TypesenseBookDoc, error)
	GetBookRatingsByIDs(ctx context.Context, ids []int) (map[int]api.APIBook, error)
	BookDetail(ctx context.Context, id int) (*api.BookDetail, error)
	Trending(ctx context.Context, duration string, limit int) ([]api.APIBook, error)

	ReadingQueue(ctx context.Context, create bool) (int, []api.RankedBook, error)
	QueueBooks(ctx context.Context, listID int, books []api.RankedBook) error
	SetQueuePositions(ctx context.Context, listID int, rows []api.RankedBook) error

	ExportLibrary(ctx context.Context) ([]api.APIUserBook, error)
	LookupISBN(ctx context.Context, isbn string) (int, error)
	ImportReads(ctx context.Context, userBookID int, reads []api.APIUserBookRead) error
}

var _ Hardcover = (*api.Client)(nil)

// ErrOffline reports that an operation needs Hardcover but the App was built
// without a client, as local-only commands are.
var ErrOffline = errors.New("Hardcover is unavailable")

// MissingPermission reports whether err is Hardcover refusing requests
// because the signed-in token lacks permissions, and if so explains which
// ones, what they are for and how to get them, without the call-site context
// wrapped around it.
func MissingPermission(err error) (string, bool) {
	missing := api.MissingScopes(err)
	if len(missing) == 0 {
		return "", false
	}
	why := api.DescribeMissingScopes(missing)
	if remedy := api.ScopeRemedy(err); remedy != "" {
		why += ". " + remedy
	}
	return why, true
}

// Online reports whether the App can reach Hardcover. A nil App is offline.
func (a *App) Online() bool {
	return a != nil && a.hc != nil
}

// requireOnline returns ErrOffline when there is no Hardcover client.
func (a *App) requireOnline() error {
	if !a.Online() {
		return ErrOffline
	}
	return nil
}
