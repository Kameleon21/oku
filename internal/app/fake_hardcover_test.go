package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/config"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/store"
)

// fakeHardcover is an in-memory Hardcover. Library rows live in userBooks
// keyed by status; inserts land there so a follow-up sync sees them. Every
// call is recorded in calls as "Method(args)" so tests can assert what was
// sent; errs fails a single method by name.
type fakeHardcover struct {
	calls []string
	errs  map[string]error
	// afterWrite runs after each library write succeeds, so a test can break
	// the local cache between the remote write and the cache update.
	afterWrite func()

	userID    int
	username  string
	privacy   int
	userBooks map[model.Status][]api.APIUserBook
	nextID    int

	// readStartedAt is what InsertUserBookRead reports as the new read's
	// start date.
	readStartedAt *string

	journals     []api.APIReadingJournal
	bookJournal  map[int][]api.JournalEntry
	goals        []api.APIGoal
	goalSettings map[int]api.GoalInput
	savedGoals   []api.GoalInput

	search  []model.SearchResult
	ratings map[int]model.Book
	details map[int]*api.BookDetail
	trend   []api.APIBook

	// queueListID is 0 until the ranked list exists. queued and
	// positionUpdates hold each QueueBooks and SetQueuePositions payload.
	queueListID     int
	queue           []api.RankedBook
	queued          [][]api.RankedBook
	positionUpdates [][]api.RankedBook

	library []api.APIUserBook
	isbns   map[string]int
}

var _ Hardcover = (*fakeHardcover)(nil)

func newFakeHardcover() *fakeHardcover {
	return &fakeHardcover{
		userID:    1,
		username:  "reader",
		privacy:   1,
		userBooks: map[model.Status][]api.APIUserBook{},
		nextID:    1000,
	}
}

// record logs a call and returns the injected error for it, if any.
func (f *fakeHardcover) record(method string, args ...any) error {
	call := method + "("
	for i, a := range args {
		if i > 0 {
			call += ", "
		}
		call += fmt.Sprint(a)
	}
	f.calls = append(f.calls, call+")")
	return f.errs[method]
}

// write is record for a library write: it also runs afterWrite when the
// write succeeds.
func (f *fakeHardcover) write(method string, args ...any) error {
	if err := f.record(method, args...); err != nil {
		return err
	}
	if f.afterWrite != nil {
		f.afterWrite()
	}
	return nil
}

// called returns the recorded calls to method, in order.
func (f *fakeHardcover) called(method string) []string {
	var out []string
	for _, c := range f.calls {
		if len(c) > len(method) && c[:len(method)+1] == method+"(" {
			out = append(out, c)
		}
	}
	return out
}

func (f *fakeHardcover) GetMe(ctx context.Context) (int, string, error) {
	if err := f.record("GetMe"); err != nil {
		return 0, "", err
	}
	return f.userID, f.username, nil
}

func (f *fakeHardcover) GetAccountPrivacySetting(ctx context.Context) (int, error) {
	if err := f.record("GetAccountPrivacySetting"); err != nil {
		return 0, err
	}
	return f.privacy, nil
}

func (f *fakeHardcover) ListUserBooks(ctx context.Context, statusID int) ([]api.APIUserBook, error) {
	if err := f.record("ListUserBooks", statusID); err != nil {
		return nil, err
	}
	return append([]api.APIUserBook(nil), f.userBooks[model.Status(statusID)]...), nil
}

func (f *fakeHardcover) InsertUserBook(ctx context.Context, bookID int, statusID int) (int, error) {
	if err := f.write("InsertUserBook", bookID, statusID); err != nil {
		return 0, err
	}
	f.nextID++
	status := model.Status(statusID)
	f.userBooks[status] = append(f.userBooks[status], api.APIUserBook{
		ID:       f.nextID,
		StatusID: statusID,
		Book:     api.APIBook{ID: bookID, Title: fmt.Sprintf("Book %d", bookID)},
	})
	return f.nextID, nil
}

func (f *fakeHardcover) UpdateUserBookStatus(ctx context.Context, userBookID int, statusID int) error {
	return f.write("UpdateUserBookStatus", userBookID, statusID)
}

func (f *fakeHardcover) UpdateUserBookRating(ctx context.Context, userBookID int, rating float64) error {
	return f.write("UpdateUserBookRating", userBookID, rating)
}

func (f *fakeHardcover) UpdateUserBookReviewAndRating(ctx context.Context, userBookID int, rating float64, review string, reviewedAt string) error {
	return f.write("UpdateUserBookReviewAndRating", userBookID, rating, review, reviewedAt)
}

func (f *fakeHardcover) InsertUserBookRead(ctx context.Context, userBookID int, progressPages int) (*api.APIUserBookRead, error) {
	if err := f.write("InsertUserBookRead", userBookID, progressPages); err != nil {
		return nil, err
	}
	f.nextID++
	return &api.APIUserBookRead{ID: f.nextID, ProgressPages: progressPages, StartedAt: f.readStartedAt}, nil
}

func (f *fakeHardcover) UpdateReadProgress(ctx context.Context, userBookReadID int, progressPages int) error {
	return f.write("UpdateReadProgress", userBookReadID, progressPages)
}

func (f *fakeHardcover) InsertProgressJournal(ctx context.Context, bookID, progressPages, totalPages, privacySettingID int) error {
	return f.record("InsertProgressJournal", bookID, progressPages, totalPages, privacySettingID)
}

func (f *fakeHardcover) ListReadingJournals(ctx context.Context, userID int, since time.Time) ([]api.APIReadingJournal, error) {
	if err := f.record("ListReadingJournals", userID); err != nil {
		return nil, err
	}
	return f.journals, nil
}

func (f *fakeHardcover) CreateJournalEntry(ctx context.Context, bookID int, event, entry string, privacy int) (int, error) {
	if err := f.record("CreateJournalEntry", bookID, event, entry, privacy); err != nil {
		return 0, err
	}
	f.nextID++
	return f.nextID, nil
}

func (f *fakeHardcover) BookJournal(ctx context.Context, bookID int) ([]api.JournalEntry, error) {
	if err := f.record("BookJournal", bookID); err != nil {
		return nil, err
	}
	return f.bookJournal[bookID], nil
}

func (f *fakeHardcover) ListGoals(ctx context.Context) ([]api.APIGoal, error) {
	if err := f.record("ListGoals"); err != nil {
		return nil, err
	}
	return f.goals, nil
}

func (f *fakeHardcover) GoalSettings(ctx context.Context, id int) (api.GoalInput, error) {
	if err := f.record("GoalSettings", id); err != nil {
		return api.GoalInput{}, err
	}
	g, ok := f.goalSettings[id]
	if !ok {
		return api.GoalInput{}, fmt.Errorf("goal %d not found in your account", id)
	}
	return g, nil
}

func (f *fakeHardcover) SaveGoal(ctx context.Context, id int, g api.GoalInput) (int, error) {
	if err := f.record("SaveGoal", id); err != nil {
		return 0, err
	}
	f.savedGoals = append(f.savedGoals, g)
	if id > 0 {
		return id, nil
	}
	f.nextID++
	return f.nextID, nil
}

func (f *fakeHardcover) SearchBooks(ctx context.Context, query string, perPage int, mode model.SearchMode) ([]model.SearchResult, error) {
	if err := f.record("SearchBooks", query, perPage, mode); err != nil {
		return nil, err
	}
	return append([]model.SearchResult(nil), f.search...), nil
}

func (f *fakeHardcover) GetBookRatingsByIDs(ctx context.Context, ids []int) (map[int]model.Book, error) {
	if err := f.record("GetBookRatingsByIDs", ids); err != nil {
		return nil, err
	}
	return f.ratings, nil
}

func (f *fakeHardcover) BookDetail(ctx context.Context, id int) (*api.BookDetail, error) {
	if err := f.record("BookDetail", id); err != nil {
		return nil, err
	}
	if d, ok := f.details[id]; ok {
		return d, nil
	}
	return &api.BookDetail{APIBook: api.APIBook{ID: id, Title: fmt.Sprintf("Book %d", id)}}, nil
}

func (f *fakeHardcover) Trending(ctx context.Context, duration string, limit int) ([]api.APIBook, error) {
	if err := f.record("Trending", duration, limit); err != nil {
		return nil, err
	}
	return f.trend, nil
}

// ReadingQueue mirrors the API: without the list it returns 0 and no rows
// unless create is set, and rows are ordered by position, then row ID.
func (f *fakeHardcover) ReadingQueue(ctx context.Context, create bool) (int, []api.RankedBook, error) {
	if err := f.record("ReadingQueue", create); err != nil {
		return 0, nil, err
	}
	if f.queueListID == 0 {
		if !create {
			return 0, []api.RankedBook{}, nil
		}
		f.queueListID = 77
	}
	rows := append([]api.RankedBook{}, f.queue...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Position != rows[j].Position {
			return rows[i].Position < rows[j].Position
		}
		return rows[i].ID < rows[j].ID
	})
	return f.queueListID, rows, nil
}

func (f *fakeHardcover) QueueBooks(ctx context.Context, listID int, books []api.RankedBook) error {
	if err := f.record("QueueBooks", listID); err != nil {
		return err
	}
	f.queued = append(f.queued, append([]api.RankedBook(nil), books...))
	for _, b := range books {
		f.nextID++
		b.ID = f.nextID
		f.queue = append(f.queue, b)
	}
	return nil
}

func (f *fakeHardcover) SetQueuePositions(ctx context.Context, listID int, rows []api.RankedBook) error {
	if err := f.record("SetQueuePositions", listID); err != nil {
		return err
	}
	f.positionUpdates = append(f.positionUpdates, append([]api.RankedBook(nil), rows...))
	for _, r := range rows {
		for i := range f.queue {
			if f.queue[i].ID == r.ID {
				f.queue[i].Position = r.Position
			}
		}
	}
	return nil
}

func (f *fakeHardcover) ExportLibrary(ctx context.Context) ([]api.APIUserBook, error) {
	if err := f.record("ExportLibrary"); err != nil {
		return nil, err
	}
	return f.library, nil
}

func (f *fakeHardcover) LookupISBN(ctx context.Context, isbn string) (int, error) {
	if err := f.record("LookupISBN", isbn); err != nil {
		return 0, err
	}
	if id, ok := f.isbns[isbn]; ok {
		return id, nil
	}
	return 0, &api.ISBNMatchError{ISBN: isbn}
}

func (f *fakeHardcover) ImportReads(ctx context.Context, userBookID int, reads []api.APIUserBookRead) error {
	return f.record("ImportReads", userBookID, len(reads))
}

// newTestAppWith returns an App backed by hc and a fresh SQLite store that is
// closed when the test ends. A nil hc gives an offline App.
func newTestAppWith(t *testing.T, hc Hardcover) *App {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	a := New(hc, db, config.Config{})
	t.Cleanup(func() { _ = a.Close() })
	return a
}
