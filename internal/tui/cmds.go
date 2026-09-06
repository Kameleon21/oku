package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	backgroundSyncWindow = 10 * time.Minute
	backgroundCheckEvery = 1 * time.Minute
)

// ── Messages ───────────────────────────────────────────────────────────────

type libraryLoadedMsg struct {
	reading      []model.UserBook
	oku          []model.UserBook
	needsRefresh bool
	// reconcile marks the background reconcile's own result: only that one
	// may clear dirty, since any other load can land while it is in flight.
	reconcile bool
	err       error
}

type searchLoadedMsg struct {
	results []model.SearchResult
	query   string
	mode    model.SearchMode
	// seq stamps the search this result belongs to; anything but the latest
	// is dropped.
	seq int
	err error
}

// opKind identifies which operation an opDoneMsg belongs to, so a modal can
// react to its own result and ignore results of other in-flight work.
type opKind int

const (
	opUnknown opKind = iota
	opProgress
	opStatus
	opReview
	opSync
)

type opDoneMsg struct {
	op opKind
	// seq identifies the modal session that started the operation.
	seq       int
	info      string
	err       error
	reload    bool
	markDirty bool

	// What a status change or a progress update did to which book, so the
	// result can offer to undo it. Zero for every other operation.
	bookID                int
	title                 string
	prevStatus, newStatus model.Status
	prevPage, newPage     int
}

type backgroundCheckMsg struct{}

type timerTickMsg time.Time

type localDataLoadedMsg struct {
	readingStats   *model.ReadingStats
	recentSessions []model.ReadingSession
	recentSearches []string
	timerState     *model.TimerState
	timerBook      *model.Book
	lastSyncAt     time.Time
	err            error
}

type timerOpDoneMsg struct {
	info    string
	err     error
	session *model.ReadingSession
}

// ── Tea Commands ───────────────────────────────────────────────────────────

func loadCachedLibraryCmd(a *app.App) tea.Cmd {
	return func() tea.Msg {
		if a == nil {
			return libraryLoadedMsg{err: fmt.Errorf("dashboard app is not initialized")}
		}
		reading, readingStale, err := a.ListCachedBooks(model.StatusCurrentlyReading)
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		oku, okuStale, err := a.ListCachedBooks(model.StatusWantToRead)
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		return libraryLoadedMsg{
			reading:      reading,
			oku:          oku,
			needsRefresh: readingStale || okuStale,
		}
	}
}

func loadLibraryCmd(ctx context.Context, a *app.App, refresh bool) tea.Cmd {
	return func() tea.Msg {
		if a == nil {
			return libraryLoadedMsg{err: fmt.Errorf("dashboard app is not initialized")}
		}
		reading, err := a.ListBooks(ctx, model.StatusCurrentlyReading, refresh)
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		oku, err := a.ListBooks(ctx, model.StatusWantToRead, refresh)
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		return libraryLoadedMsg{
			reading: reading,
			oku:     oku,
		}
	}
}

// reconcileLibraryCmd is the background reconcile's refresh: it stamps its
// result so that a library load started by something else cannot be mistaken
// for the reconcile finishing.
func reconcileLibraryCmd(ctx context.Context, a *app.App) tea.Cmd {
	refresh := loadLibraryCmd(ctx, a, true)
	return func() tea.Msg {
		msg := refresh()
		if loaded, ok := msg.(libraryLoadedMsg); ok {
			loaded.reconcile = true
			return loaded
		}
		return msg
	}
}

// recentSessionsLimit is how much timer history a local load brings in: the
// Timer pane shows five, the rest is there for a per-book view.
const recentSessionsLimit = 50

// loadLocalDataCmd reads everything the dashboard shows from the local
// store. now is the clock the demo data is built around.
func loadLocalDataCmd(a *app.App, now func() time.Time) tea.Cmd {
	return func() tea.Msg {
		if a == nil {
			return localDataLoadedMsg{err: fmt.Errorf("app not initialized")}
		}
		stats, err := a.GetReadingStats()
		if err != nil {
			return localDataLoadedMsg{err: err}
		}
		sessions, err := a.TimerList(recentSessionsLimit)
		if err != nil {
			return localDataLoadedMsg{err: err}
		}
		timer, err := a.TimerStatus()
		if err != nil {
			return localDataLoadedMsg{err: err}
		}

		// Resolved here, off the render path: View must not query the store.
		var timerBook *model.Book
		if timer != nil && timer.BookID > 0 {
			if b, err := a.Store.GetBookByID(timer.BookID); err == nil {
				timerBook = b
			}
		}

		if shouldUseDemoLocalData() {
			stats, sessions = demoLocalData(now())
		}

		// Best effort: an unreadable history is not a reason to fail the load.
		var recentSearches []string
		if a.Store != nil {
			if raw, err := a.Store.GetState(recentSearchesKey); err == nil {
				recentSearches = decodeRecentSearches(raw)
			}
		}

		return localDataLoadedMsg{
			readingStats:   stats,
			recentSessions: sessions,
			recentSearches: recentSearches,
			timerState:     timer,
			timerBook:      timerBook,
			lastSyncAt:     a.LastSyncAt(),
		}
	}
}

// saveRecentSearchesCmd persists the search history off the update loop. It is
// best effort: a failing write must not interrupt a search.
func saveRecentSearchesCmd(a *app.App, queries []string) tea.Cmd {
	if a == nil || a.Store == nil {
		return nil
	}
	snapshot := append([]string(nil), queries...)
	return func() tea.Msg {
		if raw, err := encodeRecentSearches(snapshot); err == nil {
			_ = a.Store.SetState(recentSearchesKey, raw)
		}
		return nil
	}
}

func searchCmd(ctx context.Context, a *app.App, query string, mode model.SearchMode, seq int) tea.Cmd {
	return func() tea.Msg {
		results, err := a.SearchBooks(ctx, query, 10, mode)
		return searchLoadedMsg{
			results: results,
			query:   query,
			mode:    mode,
			seq:     seq,
			err:     err,
		}
	}
}

// updateProgressCmd sets a book's page. prevPage is where the book stood, so
// the result can offer to put it back.
func updateProgressCmd(ctx context.Context, a *app.App, bookID int, title string, prevPage int, rawPage string) tea.Cmd {
	return func() tea.Msg {
		p, err := model.ParsePage(rawPage)
		if err != nil {
			return opDoneMsg{op: opProgress, err: err}
		}
		newPage, err := a.UpdateProgress(ctx, bookID, p)
		if err != nil {
			return opDoneMsg{op: opProgress, err: err}
		}
		return opDoneMsg{
			op:        opProgress,
			info:      fmt.Sprintf("Progress updated to page %d", newPage),
			reload:    true,
			markDirty: true,
			bookID:    bookID,
			title:     title,
			prevPage:  prevPage,
			newPage:   newPage,
		}
	}
}

func submitReviewRatingCmd(ctx context.Context, a *app.App, bookID int, rating float64, review string) tea.Cmd {
	return func() tea.Msg {
		if err := a.ReviewBook(ctx, bookID, rating, review); err != nil {
			return opDoneMsg{op: opReview, err: err}
		}
		info := fmt.Sprintf("Updated review and rating (%s)", model.StarString(rating))
		if strings.TrimSpace(review) == "" {
			info = fmt.Sprintf("Updated rating (%s)", model.StarString(rating))
		}
		return opDoneMsg{
			op:        opReview,
			info:      info,
			reload:    true,
			markDirty: true,
		}
	}
}

// stamped marks an operation's result with the modal session that started
// it, so the modal can tell its own result from any other.
func stamped(cmd tea.Cmd, seq int) tea.Cmd {
	return func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(opDoneMsg); ok {
			done.seq = seq
			return done
		}
		return msg
	}
}

func reviewSavePendingMessage(review string) string {
	if strings.TrimSpace(review) == "" {
		return "Saving rating..."
	}
	return "Saving review..."
}

// quickProgressCmd moves a book's page by delta. prevPage is where the book
// stood, so the result can offer to put it back.
func quickProgressCmd(ctx context.Context, a *app.App, bookID int, title string, prevPage, delta int) tea.Cmd {
	return func() tea.Msg {
		if delta == 0 {
			return opDoneMsg{op: opProgress}
		}
		newPage, err := a.UpdateProgress(ctx, bookID, model.PageUpdate{
			Delta:    delta,
			Relative: true,
		})
		if err != nil {
			return opDoneMsg{op: opProgress, err: err}
		}
		sign := ""
		if delta > 0 {
			sign = "+"
		}
		return opDoneMsg{
			op:        opProgress,
			info:      fmt.Sprintf("Progress %s%d → page %d", sign, delta, newPage),
			reload:    true,
			markDirty: true,
			bookID:    bookID,
			title:     title,
			prevPage:  prevPage,
			newPage:   newPage,
		}
	}
}

// changeStatusCmd moves a book from one shelf to another. Both are reported
// so the result can offer to move it back.
func changeStatusCmd(ctx context.Context, a *app.App, bookID int, title string, from, to model.Status) tea.Cmd {
	return func() tea.Msg {
		if err := a.ChangeStatus(ctx, bookID, to); err != nil {
			return opDoneMsg{op: opStatus, err: err}
		}
		return opDoneMsg{
			op:         opStatus,
			info:       fmt.Sprintf("Status changed to %s", to.Label()),
			reload:     true,
			markDirty:  true,
			bookID:     bookID,
			title:      title,
			prevStatus: from,
			newStatus:  to,
		}
	}
}

func addFromSearchCmd(ctx context.Context, a *app.App, bookID int, status model.Status) tea.Cmd {
	return func() tea.Msg {
		if err := a.ChangeStatus(ctx, bookID, status); err != nil {
			return opDoneMsg{op: opStatus, err: err}
		}
		return opDoneMsg{
			op:        opStatus,
			info:      fmt.Sprintf("Added to %s", status.Label()),
			reload:    true,
			markDirty: true,
		}
	}
}

func syncAllAndReloadCmd(ctx context.Context, a *app.App) tea.Cmd {
	return func() tea.Msg {
		if err := a.SyncAll(ctx); err != nil {
			return opDoneMsg{op: opSync, err: err}
		}
		return opDoneMsg{
			op:     opSync,
			info:   "Sync complete",
			reload: true,
		}
	}
}

func backgroundCheckCmd() tea.Cmd {
	return tea.Tick(backgroundCheckEvery, func(time.Time) tea.Msg {
		return backgroundCheckMsg{}
	})
}

func timerTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return timerTickMsg(t)
	})
}

func startTimerForBookCmd(a *app.App, bookID int) tea.Cmd {
	return func() tea.Msg {
		if a == nil {
			return timerOpDoneMsg{err: fmt.Errorf("app not initialized")}
		}
		if bookID <= 0 {
			return timerOpDoneMsg{err: fmt.Errorf("invalid book selection")}
		}

		if err := a.TimerStart(bookID); err != nil {
			return timerOpDoneMsg{err: err}
		}

		info := "Timer started"
		if bookID > 0 {
			if b, err := a.Store.GetBookByID(bookID); err == nil && b != nil {
				info = fmt.Sprintf("Timer started — %s", b.Title)
			}
		}
		return timerOpDoneMsg{info: info}
	}
}

func stopTimerCmd(a *app.App) tea.Cmd {
	return func() tea.Msg {
		if a == nil {
			return timerOpDoneMsg{err: fmt.Errorf("app not initialized")}
		}
		session, err := a.TimerStop()
		if err != nil {
			return timerOpDoneMsg{err: err}
		}
		return timerOpDoneMsg{
			info:    fmt.Sprintf("Session complete — %s", format.Duration(session.Duration())),
			session: session,
		}
	}
}

func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}
