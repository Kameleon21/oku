package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
)

// fakeBackend is an in-memory backend: a library keyed by status, the local
// data the dashboard reads, and a record of every call. online says whether
// Hardcover answers; when it does not, every remote method reports
// app.ErrOffline the way App does. errs fails one method by name, and a
// write failed with app.ErrCacheRefresh still lands, since that error means
// Hardcover took it. The program runs commands in parallel, so the methods
// lock; a test reads the fields directly once the model is at rest.
type fakeBackend struct {
	mu     sync.Mutex
	online bool
	calls  []string
	errs   map[string]error

	books map[model.Status][]model.UserBook
	queue []int
	// stale makes ListCachedBooks report the cache out of date, which asks
	// the dashboard for a refresh.
	stale bool

	stats    *model.ReadingStats
	notes    map[int]string
	history  string
	sessions []model.ReadingSession
	timer    *model.TimerState
	lastSync time.Time
	now      func() time.Time

	results  []model.SearchResult
	trending []model.SearchResult
	details  map[int]*model.BookDetail
	journals map[int][]model.BookJournalEntry
	nextID   int
}

var _ backend = (*fakeBackend)(nil)

// newFakeBackend is an online backend with nothing in it.
func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		online:   true,
		errs:     map[string]error{},
		books:    map[model.Status][]model.UserBook{},
		stats:    &model.ReadingStats{},
		notes:    map[int]string{},
		details:  map[int]*model.BookDetail{},
		journals: map[int][]model.BookJournalEntry{},
		now:      time.Now,
		nextID:   1000,
	}
}

// add shelves books on the status each one carries.
func (f *fakeBackend) add(books ...model.UserBook) {
	for _, b := range books {
		if b.BookID == 0 {
			b.BookID = b.Book.ID
		}
		f.books[b.StatusID] = append(f.books[b.StatusID], b)
	}
}

// called reports whether a call to method was recorded, starting with the
// arguments given when there are any: called("ChangeStatus", 1) matches
// "ChangeStatus(1, finished)".
func (f *fakeBackend) called(method string, args ...any) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := strings.TrimSuffix(describe(method, args...), ")")
	for _, c := range f.calls {
		if strings.HasPrefix(c, want) {
			return true
		}
	}
	return false
}

func describe(method string, args ...any) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = fmt.Sprint(a)
	}
	return method + "(" + strings.Join(parts, ", ") + ")"
}

// call records a local call and returns the error injected for it.
func (f *fakeBackend) call(method string, args ...any) error {
	f.calls = append(f.calls, describe(method, args...))
	return f.errs[method]
}

// remote records a call that needs Hardcover: offline beats any injected
// error, the way App refuses before it tries.
func (f *fakeBackend) remote(method string, args ...any) error {
	err := f.call(method, args...)
	if !f.online {
		return app.ErrOffline
	}
	return err
}

// write is remote for a mutation: an injected ErrCacheRefresh is handed back
// to the caller after apply has run, any other error before.
func (f *fakeBackend) write(apply func(), method string, args ...any) error {
	err := f.remote(method, args...)
	if err != nil && !errors.Is(err, app.ErrCacheRefresh) {
		return err
	}
	apply()
	return err
}

func (f *fakeBackend) Online() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.online
}

func (f *fakeBackend) ListCachedBooks(status model.Status) ([]model.UserBook, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListCachedBooks", status); err != nil {
		return nil, false, err
	}
	return f.shelf(status), f.stale, nil
}

func (f *fakeBackend) CachedBooks(status model.Status) ([]model.UserBook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CachedBooks", status); err != nil {
		return nil, err
	}
	return f.shelf(status), nil
}

// ListBooks only reaches for Hardcover when asked to refresh; App does the
// same when the cache is fresh.
func (f *fakeBackend) ListBooks(ctx context.Context, status model.Status, refresh bool) ([]model.UserBook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var err error
	if refresh {
		err = f.remote("ListBooks", status, refresh)
	} else {
		err = f.call("ListBooks", status, refresh)
	}
	if err != nil {
		return nil, err
	}
	return f.shelf(status), nil
}

func (f *fakeBackend) ListAllCachedUserBooks() ([]model.UserBook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListAllCachedUserBooks"); err != nil {
		return nil, err
	}
	var all []model.UserBook
	for _, status := range model.AllStatuses {
		all = append(all, f.shelf(status)...)
	}
	return all, nil
}

func (f *fakeBackend) CachedBook(id int) (*model.Book, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CachedBook", id); err != nil {
		return nil, err
	}
	if b := f.find(id); b != nil {
		book := cloneBook(*b).Book
		return &book, nil
	}
	return nil, nil
}

func (f *fakeBackend) CachedQueueOrder() ([]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("CachedQueueOrder"); err != nil {
		return nil, err
	}
	return append([]int(nil), f.queue...), nil
}

func (f *fakeBackend) LastSyncAt() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastSync
}

func (f *fakeBackend) SyncAll(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.write(func() { f.lastSync = f.now() }, "SyncAll")
}

func (f *fakeBackend) ChangeStatus(ctx context.Context, bookID int, status model.Status) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.write(func() {
		b := f.take(bookID)
		if b == nil {
			b = &model.UserBook{ID: f.id(), BookID: bookID, Book: model.Book{ID: bookID}}
		}
		b.StatusID = status
		f.books[status] = append(f.books[status], *b)
	}, "ChangeStatus", bookID, status)
}

func (f *fakeBackend) UpdateProgress(ctx context.Context, bookID int, update model.PageUpdate) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	page := 0
	err := f.write(func() {
		b := f.find(bookID)
		if b == nil {
			return
		}
		if update.Relative {
			page = currentPage(*b) + update.Delta
		} else {
			page = update.Absolute
		}
		b.CurrentPage = page
		if len(b.UserBookReads) > 0 {
			b.UserBookReads[0].ProgressPages = page
		}
	}, "UpdateProgress", bookID, update)
	return page, err
}

func (f *fakeBackend) ReviewBook(ctx context.Context, bookID int, rating float64, review string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.write(func() {
		if b := f.find(bookID); b != nil {
			b.Rating, b.Review = rating, review
		}
	}, "ReviewBook", bookID, rating, review)
}

func (f *fakeBackend) AddJournalEntry(ctx context.Context, bookID int, event, text string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := 0
	err := f.write(func() {
		id = f.id()
		f.journals[bookID] = append(f.journals[bookID], model.BookJournalEntry{Event: event, Entry: text})
	}, "AddJournalEntry", bookID, event, text)
	return id, err
}

func (f *fakeBackend) MoveQueueBook(ctx context.Context, bookID, delta int) ([]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	err := f.write(func() {
		for i, id := range f.queue {
			if id != bookID {
				continue
			}
			j := max(0, min(len(f.queue)-1, i+delta))
			f.queue[i], f.queue[j] = f.queue[j], f.queue[i]
			break
		}
	}, "MoveQueueBook", bookID, delta)
	if err != nil {
		return nil, err
	}
	return append([]int(nil), f.queue...), nil
}

func (f *fakeBackend) RefreshQueue(ctx context.Context) ([]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.remote("RefreshQueue"); err != nil {
		return nil, err
	}
	return append([]int(nil), f.queue...), nil
}

func (f *fakeBackend) SearchBooks(ctx context.Context, query string, limit int, mode model.SearchMode) ([]model.SearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.remote("SearchBooks", query, limit, mode); err != nil {
		return nil, err
	}
	return f.results, nil
}

func (f *fakeBackend) TrendingBooks(ctx context.Context, period string, limit int, raw bool) ([]model.SearchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.remote("TrendingBooks", period, limit, raw); err != nil {
		return nil, err
	}
	return f.trending, nil
}

func (f *fakeBackend) GetBookDetail(ctx context.Context, id int, refresh bool) (*model.BookDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.remote("GetBookDetail", id, refresh); err != nil {
		return nil, err
	}
	if d, ok := f.details[id]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("book %d not found", id)
}

func (f *fakeBackend) BookJournal(ctx context.Context, bookID int) ([]model.BookJournalEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.remote("BookJournal", bookID); err != nil {
		return nil, err
	}
	return append([]model.BookJournalEntry(nil), f.journals[bookID]...), nil
}

func (f *fakeBackend) GetReadingStats() (*model.ReadingStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("GetReadingStats"); err != nil {
		return nil, err
	}
	return f.stats, nil
}

func (f *fakeBackend) ListPrivateNotes() (map[int]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("ListPrivateNotes"); err != nil {
		return nil, err
	}
	notes := make(map[int]string, len(f.notes))
	for id, text := range f.notes {
		notes[id] = text
	}
	return notes, nil
}

func (f *fakeBackend) SavePrivateNote(bookID int, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("SavePrivateNote", bookID, text); err != nil {
		return err
	}
	f.notes[bookID] = text
	return nil
}

func (f *fakeBackend) RecentSearches() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("RecentSearches"); err != nil {
		return "", err
	}
	return f.history, nil
}

func (f *fakeBackend) SaveRecentSearches(raw string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("SaveRecentSearches", raw); err != nil {
		return err
	}
	f.history = raw
	return nil
}

func (f *fakeBackend) TimerStart(bookID int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("TimerStart", bookID); err != nil {
		return err
	}
	if f.timer != nil {
		return fmt.Errorf("timer already running (started %s)", f.timer.StartedAt.Local().Format("3:04 PM"))
	}
	f.timer = &model.TimerState{BookID: bookID, StartedAt: f.now()}
	return nil
}

func (f *fakeBackend) TimerStop() (*model.ReadingSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("TimerStop"); err != nil {
		return nil, err
	}
	if f.timer == nil {
		return nil, fmt.Errorf("no timer running")
	}
	ended := f.now()
	s := model.ReadingSession{ID: f.id(), BookID: f.timer.BookID, StartedAt: f.timer.StartedAt, EndedAt: &ended}
	if b := f.find(s.BookID); b != nil {
		s.BookTitle = b.Book.Title
	}
	f.sessions = append([]model.ReadingSession{s}, f.sessions...)
	f.timer = nil
	return &s, nil
}

func (f *fakeBackend) TimerStatus() (*model.TimerState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("TimerStatus"); err != nil {
		return nil, err
	}
	return f.timer, nil
}

func (f *fakeBackend) TimerList(limit int) ([]model.ReadingSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("TimerList", limit); err != nil {
		return nil, err
	}
	if len(f.sessions) > limit {
		return f.sessions[:limit], nil
	}
	return f.sessions, nil
}

// shelf is a copy of one status's books, so a caller cannot reach into the
// fake's own rows.
func (f *fakeBackend) shelf(status model.Status) []model.UserBook {
	out := make([]model.UserBook, 0, len(f.books[status]))
	for _, b := range f.books[status] {
		out = append(out, cloneBook(b))
	}
	return out
}

// cloneBook is a user book with nothing shared with the fake's own row: the
// reads, their dates and the authors are copied, so a test sees a change
// only once a reload has brought it in, and a running program never reads
// what a write is changing.
func cloneBook(b model.UserBook) model.UserBook {
	b.Book.Authors = append([]string(nil), b.Book.Authors...)
	b.ReviewedAt = cloneTime(b.ReviewedAt)
	if b.UserBookReads != nil {
		reads := make([]model.UserBookRead, len(b.UserBookReads))
		for i, r := range b.UserBookReads {
			r.StartedAt, r.FinishedAt = cloneTime(r.StartedAt), cloneTime(r.FinishedAt)
			reads[i] = r
		}
		b.UserBookReads = reads
	}
	return b
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}

// find is the fake's own row for a book, on whichever shelf it is.
func (f *fakeBackend) find(bookID int) *model.UserBook {
	for status := range f.books {
		for i := range f.books[status] {
			if f.books[status][i].BookID == bookID {
				return &f.books[status][i]
			}
		}
	}
	return nil
}

// take removes a book from its shelf and returns it.
func (f *fakeBackend) take(bookID int) *model.UserBook {
	for status, books := range f.books {
		for i, b := range books {
			if b.BookID == bookID {
				f.books[status] = append(books[:i:i], books[i+1:]...)
				return &b
			}
		}
	}
	return nil
}

func (f *fakeBackend) id() int {
	f.nextID++
	return f.nextID
}
