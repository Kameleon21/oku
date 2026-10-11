package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
)

const (
	backgroundSyncWindow = 10 * time.Minute
	backgroundCheckEvery = 1 * time.Minute
)

// ── Messages ───────────────────────────────────────────────────────────────

type libraryLoadedMsg struct {
	reading      []model.UserBook
	oku          []model.UserBook
	paused       []model.UserBook
	queueOrder   []int
	needsRefresh bool
	// reconcile marks the background reconcile's own result: only that one
	// may clear dirty, since any other load can land while it is in flight.
	reconcile bool
	// quiet marks a load started by a warned write (see quietly): its
	// failure leaves the warning up rather than replacing it.
	quiet bool
	err   error
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
	opJournal
	opPrivateNote
)

type opDoneMsg struct {
	op opKind
	// seq identifies the modal session that started the operation.
	seq         int
	info        string
	err         error
	reload      bool
	markDirty   bool
	privateNote string

	// warn is set when a change reached Hardcover but the local cache could
	// not be refreshed. The result is otherwise a success.
	warn string

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
	privateNotes   map[int]string
	recentSessions []model.ReadingSession
	recentSearches []string
	timerState     *model.TimerState
	timerBook      *model.Book
	lastSyncAt     time.Time
	// shelf is every cached user book by book id: what the search detail
	// reads to say a result is already in the library.
	shelf map[int]model.UserBook
	// quiet marks a load started by a warned write (see quietly).
	quiet bool
	err   error
}

type timerOpDoneMsg struct {
	info    string
	err     error
	session *model.ReadingSession
}

// ── Tea Commands ───────────────────────────────────────────────────────────

// loadCachedLibraryCmd is the launch load: the cached shelves at once, then,
// when Hardcover can be reached, a refresh of all three whatever their age,
// so changes made on the website show within seconds of opening.
func loadCachedLibraryCmd(a backend) tea.Cmd {
	return func() tea.Msg {
		reading, err := a.CachedBooks(model.StatusCurrentlyReading)
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		oku, err := a.CachedBooks(model.StatusWantToRead)
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		paused, err := a.CachedBooks(model.StatusPaused)
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		order, err := a.CachedQueueOrder()
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		return libraryLoadedMsg{paused: paused, queueOrder: order,
			reading:      reading,
			oku:          oku,
			needsRefresh: a.Online(),
		}
	}
}

func loadLibraryCmd(ctx context.Context, a backend, refresh bool) tea.Cmd {
	return func() tea.Msg {
		reading, err := a.ListBooks(ctx, model.StatusCurrentlyReading, refresh)
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		oku, err := a.ListBooks(ctx, model.StatusWantToRead, refresh)
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		// Paused is fetched only by a forced refresh. A reload after a write
		// reads it from the cache, which the write has already patched: the
		// shelf is usually empty, and ListBooks would refetch an empty
		// shelf on every reload.
		paused, err := a.CachedBooks(model.StatusPaused)
		if refresh {
			paused, err = a.ListBooks(ctx, model.StatusPaused, true)
		}
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		order, err := a.CachedQueueOrder()
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		return libraryLoadedMsg{paused: paused, queueOrder: order,
			reading: reading,
			oku:     oku,
		}
	}
}

// reconcileLibraryCmd is the background reconcile's refresh: it stamps its
// result so that a library load started by something else cannot be mistaken
// for the reconcile finishing.
func reconcileLibraryCmd(ctx context.Context, a backend) tea.Cmd {
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

// quietly marks a load started by a write that reached Hardcover but could
// not refresh the cache: the cache may still be broken, so the load's own
// failure leaves the warning that already says to sync, and its undo, up
// instead of replacing them with an error toast. Only the marked load is
// quiet; an unrelated load failing meanwhile still reports itself.
func quietly(cmd tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case libraryLoadedMsg:
			msg.quiet = true
			return msg
		case localDataLoadedMsg:
			msg.quiet = true
			return msg
		default:
			return msg
		}
	}
}

// recentSessionsLimit is how much timer history a local load brings in: the
// Timer pane shows five, the rest is there for a per-book view.
const recentSessionsLimit = 50

// loadLocalDataCmd reads everything the dashboard shows from the local
// store. now is the clock the demo data is built around.
func loadLocalDataCmd(a backend, now func() time.Time) tea.Cmd {
	return func() tea.Msg {
		stats, err := a.GetReadingStats()
		if err != nil {
			return localDataLoadedMsg{err: err}
		}
		notes, err := a.ListPrivateNotes()
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
		var timerState *model.TimerState
		var timerBook *model.Book
		if timer != nil {
			timerState, timerBook = &timer.TimerState, timer.Book
		}

		if shouldUseDemoLocalData() {
			stats, sessions = demoLocalData(now())
		}

		// Best effort, both of them: neither the search history nor the
		// shelf lookup is worth failing the whole load over.
		shelf := map[int]model.UserBook{}
		if all, err := a.ListAllCachedUserBooks(); err == nil {
			for _, b := range all {
				shelf[b.Book.ID] = b
			}
		}

		var recentSearches []string
		if raw, err := a.RecentSearches(); err == nil {
			recentSearches = decodeRecentSearches(raw)
		}

		return localDataLoadedMsg{
			readingStats:   stats,
			privateNotes:   notes,
			recentSessions: sessions,
			recentSearches: recentSearches,
			timerState:     timerState,
			timerBook:      timerBook,
			lastSyncAt:     a.LastSyncAt(),
			shelf:          shelf,
		}
	}
}

// saveRecentSearchesCmd persists the search history off the update loop. It is
// best effort: a failing write must not interrupt a search.
func saveRecentSearchesCmd(a backend, queries []string) tea.Cmd {
	snapshot := append([]string(nil), queries...)
	return func() tea.Msg {
		if raw, err := encodeRecentSearches(snapshot); err == nil {
			_ = a.SaveRecentSearches(raw)
		}
		return nil
	}
}

func searchCmd(ctx context.Context, a backend, query string, mode model.SearchMode, seq int) tea.Cmd {
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
func updateProgressCmd(ctx context.Context, a backend, bookID int, title string, prevPage int, rawPage string) tea.Cmd {
	return func() tea.Msg {
		p, err := model.ParsePage(rawPage)
		if err != nil {
			return opDoneMsg{op: opProgress, err: err}
		}
		newPage, err := a.UpdateProgress(ctx, bookID, p)
		return writeDone(err, opDoneMsg{
			op:        opProgress,
			info:      fmt.Sprintf("Progress updated to page %d", newPage),
			reload:    true,
			markDirty: true,
			bookID:    bookID,
			title:     title,
			prevPage:  prevPage,
			newPage:   newPage,
		})
	}
}

func submitReviewRatingCmd(ctx context.Context, a backend, bookID int, rating float64, review string) tea.Cmd {
	return func() tea.Msg {
		err := a.ReviewBook(ctx, bookID, rating, review)
		info := fmt.Sprintf("Updated review and rating (%s)", model.StarString(rating))
		if strings.TrimSpace(review) == "" {
			info = fmt.Sprintf("Updated rating (%s)", model.StarString(rating))
		}
		return writeDone(err, opDoneMsg{
			op:        opReview,
			info:      info,
			reload:    true,
			markDirty: true,
		})
	}
}

// cacheWarning is the toast's note for a change that reached Hardcover while
// the local cache could not be refreshed.
const cacheWarning = "press s to sync"

// writeDone is the result of a write-through mutation: ok when it worked,
// ok with a warning when only the local cache lags behind Hardcover (the
// change stands, so it reloads, marks dirty and offers undo all the same),
// and a plain failure otherwise.
func writeDone(err error, ok opDoneMsg) opDoneMsg {
	switch {
	case err == nil:
		return ok
	case errors.Is(err, app.ErrCacheRefresh):
		ok.warn = cacheWarning
		return ok
	}
	return opDoneMsg{op: ok.op, err: err}
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
func quickProgressCmd(ctx context.Context, a backend, bookID int, title string, prevPage, delta int) tea.Cmd {
	return func() tea.Msg {
		if delta == 0 {
			return opDoneMsg{op: opProgress}
		}
		newPage, err := a.UpdateProgress(ctx, bookID, model.PageUpdate{
			Delta:    delta,
			Relative: true,
		})
		sign := ""
		if delta > 0 {
			sign = "+"
		}
		return writeDone(err, opDoneMsg{
			op:        opProgress,
			info:      fmt.Sprintf("Progress %s%d → page %d", sign, delta, newPage),
			reload:    true,
			markDirty: true,
			bookID:    bookID,
			title:     title,
			prevPage:  prevPage,
			newPage:   newPage,
		})
	}
}

// changeStatusCmd moves a book from one shelf to another. Both are reported
// so the result can offer to move it back.
func changeStatusCmd(ctx context.Context, a backend, bookID int, title string, from, to model.Status) tea.Cmd {
	return func() tea.Msg {
		err := a.ChangeStatus(ctx, bookID, to)
		return writeDone(err, opDoneMsg{
			op:         opStatus,
			info:       fmt.Sprintf("Status changed to %s", to.Label()),
			reload:     true,
			markDirty:  true,
			bookID:     bookID,
			title:      title,
			prevStatus: from,
			newStatus:  to,
		})
	}
}

func addFromSearchCmd(ctx context.Context, a backend, bookID int, status model.Status) tea.Cmd {
	return func() tea.Msg {
		err := a.ChangeStatus(ctx, bookID, status)
		return writeDone(err, opDoneMsg{
			op:        opStatus,
			info:      fmt.Sprintf("Added to %s", status.Label()),
			reload:    true,
			markDirty: true,
		})
	}
}

// bookDetailCmd fetches the rich detail of one book, and bookJournalCmd the
// reader's own notes and quotes on it; both land in the detail pane.
func bookDetailCmd(ctx context.Context, a backend, id int) tea.Cmd {
	return func() tea.Msg {
		b, err := a.GetBookDetail(ctx, id, false)
		return detailLoadedMsg{id, b, err}
	}
}

func bookJournalCmd(ctx context.Context, a backend, id int) tea.Cmd {
	return func() tea.Msg {
		entries, err := a.BookJournal(ctx, id)
		return journalLoadedMsg{id, entries, err}
	}
}

// pausedShelfCmd refreshes the paused shelf from Hardcover.
func pausedShelfCmd(ctx context.Context, a backend) tea.Cmd {
	return func() tea.Msg {
		books, err := a.ListBooks(ctx, model.StatusPaused, true)
		return pausedLoadedMsg{books, err}
	}
}

// queueMoveCmd moves a book up or down the reading queue; queueRefreshCmd
// fetches the queue as Hardcover has it.
func queueMoveCmd(ctx context.Context, a backend, bookID, delta int) tea.Cmd {
	return func() tea.Msg {
		ids, err := a.MoveQueueBook(ctx, bookID, delta)
		return queueLoadedMsg{ids, bookID, err}
	}
}

func queueRefreshCmd(ctx context.Context, a backend) tea.Cmd {
	return func() tea.Msg {
		ids, err := a.RefreshQueue(ctx)
		return queueLoadedMsg{ids: ids, err: err}
	}
}

// trendingCmd fetches this week's trending books as a search result, stamped
// with the search section's seq like any other.
func trendingCmd(ctx context.Context, a backend, seq int) tea.Cmd {
	return func() tea.Msg {
		books, err := a.TrendingBooks(ctx, "week", 20, false)
		return searchLoadedMsg{results: books, query: "Trending this week", mode: model.SearchModeBook, seq: seq, err: err}
	}
}

// savePrivateNoteCmd keeps a note on this device; token marks the result as
// the note modal's own.
func savePrivateNoteCmd(a backend, bookID, token int, text string) tea.Cmd {
	return func() tea.Msg {
		err := a.SavePrivateNote(bookID, text)
		return opDoneMsg{op: opPrivateNote, seq: token, bookID: bookID, privateNote: text, err: err, info: "Private note saved on this device"}
	}
}

// addJournalEntryCmd posts a note or a quote to the book's journal on
// Hardcover; token marks the result as the journal modal's own.
func addJournalEntryCmd(ctx context.Context, a backend, bookID, token int, event, text string) tea.Cmd {
	return func() tea.Msg {
		_, err := a.AddJournalEntry(ctx, bookID, event, text)
		return opDoneMsg{op: opJournal, seq: token, err: err, info: "Saved " + event}
	}
}

func syncAllAndReloadCmd(ctx context.Context, a backend) tea.Cmd {
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

func startTimerForBookCmd(a backend, bookID int) tea.Cmd {
	return func() tea.Msg {
		if bookID <= 0 {
			return timerOpDoneMsg{err: fmt.Errorf("invalid book selection")}
		}

		timer, err := a.TimerStart(bookID)
		if err != nil {
			return timerOpDoneMsg{err: err}
		}

		info := "Timer started"
		if timer.Book != nil {
			info = fmt.Sprintf("Timer started — %s", timer.Book.Title)
		}
		return timerOpDoneMsg{info: info}
	}
}

func stopTimerCmd(a backend) tea.Cmd {
	return func() tea.Msg {
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
