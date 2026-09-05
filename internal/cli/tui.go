package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

// ── View & Focus types ─────────────────────────────────────────────────────

type viewMode int

const (
	modeLibrary viewMode = iota
	modeUpdatePage
	modeReviewRating
)

const (
	backgroundSyncWindow = 10 * time.Minute
	backgroundCheckEvery = 1 * time.Minute
)

// pagePromptRows is how many rows View spends on the page-update prompt it
// draws under the layout: the book, where it stands, and the input.
const pagePromptRows = 3

// recentSearchesKey is the store state key the search history is kept under,
// and maxRecentSearches caps how much of it is remembered.
const (
	recentSearchesKey = "recent_searches"
	maxRecentSearches = 10
)

const (
	helpModalWidth = 50
	// helpModalChromeRows counts the modal rows that are not the scrollable
	// body: the title, the blank line under it, the footer, the padding and
	// the border.
	helpModalChromeRows = 7
	// helpModalMinBodyRows keeps the body scrollable on a very short terminal.
	helpModalMinBodyRows = 3
	// helpModalMarginRows keeps a row of dashboard above and below the modal,
	// so it reads as something laid over the screen rather than as the screen.
	helpModalMarginRows = 2
)

// minHelpBarWidth keeps the footer hints readable on a very narrow terminal.
const minHelpBarWidth = 20

// minStatusMessageWidth is the narrowest a status message may be cut to before
// it is dropped instead: an ellipsis and a couple of letters say nothing.
const minStatusMessageWidth = 8

type focusSection int

const (
	sectionIntro focusSection = iota
	sectionReading
	sectionOku
	sectionSearch
	sectionStats
	sectionTimer
	sectionCount // sentinel
)

type searchSubFocus int

const (
	searchSubInput searchSubFocus = iota
	searchSubResults
)

type searchInputMode int

const (
	searchModeNormal searchInputMode = iota
	searchModeInsert
)

type dashboardReviewFocus int

const (
	dashboardReviewFocusRating dashboardReviewFocus = iota
	dashboardReviewFocusText
)

type sectionDef struct {
	id    focusSection
	label string
	count int // -1 = no count
}

// ── List item types ────────────────────────────────────────────────────────

type userBookItem struct {
	book    model.UserBook
	density outputDensity
}

func (i userBookItem) Title() string {
	return i.book.Book.Title
}

func (i userBookItem) Description() string {
	author := i.book.Book.AuthorString()
	if author == "" {
		author = "Unknown author"
	}
	progress := i.book.Progress()
	if i.book.Book.Pages > 0 {
		page := i.book.CurrentPage
		if len(i.book.UserBookReads) > 0 {
			page = i.book.UserBookReads[0].ProgressPages
		}
		progress += " " + miniProgressBar(page, i.book.Book.Pages, 8)
	}

	switch i.density {
	case densityCompact:
		return progress
	case densityVerbose:
		if meta := bookMetaLine(i.book.Book); meta != "" {
			return fmt.Sprintf("%s · %s · %s", author, progress, meta)
		}
		return fmt.Sprintf("%s · %s", author, progress)
	default:
		return fmt.Sprintf("%s · %s", author, progress)
	}
}

func (i userBookItem) FilterValue() string {
	return i.book.Book.Title + " " + i.book.Book.AuthorString()
}

type searchResultItem struct {
	result  model.SearchResult
	density outputDensity
}

func (i searchResultItem) Title() string {
	if i.result.Pages > 0 {
		return fmt.Sprintf("%s (%d pages)", i.result.Title, i.result.Pages)
	}
	return i.result.Title
}

func (i searchResultItem) Description() string {
	author := strings.Join(i.result.Authors, ", ")
	if author == "" {
		author = "Unknown author"
	}
	rating := "★ n/a"
	if i.result.Rating > 0 {
		rating = fmt.Sprintf("★ %.2f", i.result.Rating)
		if i.result.Ratings > 0 {
			rating += fmt.Sprintf(" (%s ratings)", formatCount(i.result.Ratings))
		}
	}

	switch i.density {
	case densityCompact:
		return author
	case densityDefault:
		return rating + " · " + author
	case densityVerbose:
		metaParts := make([]string, 0, 3)
		if i.result.Pages > 0 {
			metaParts = append(metaParts, fmt.Sprintf("%d pages", i.result.Pages))
		}
		metaParts = append(metaParts, fmt.Sprintf("ID: %d", i.result.ID))
		if i.result.Slug != "" {
			metaParts = append(metaParts, "slug:"+i.result.Slug)
		}
		meta := strings.Join(metaParts, " · ")
		return rating + "\n" + author + "\n" + meta
	default:
		return fmt.Sprintf("%s | ID: %d", author, i.result.ID)
	}
}

func (i searchResultItem) FilterValue() string {
	return i.result.Title + " " + strings.Join(i.result.Authors, " ")
}

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

// undoAction reverses the operation a toast reports. It is data rather than
// a command so the dashboard can say what it will do, and a test can see it.
type undoAction struct {
	op     opKind
	bookID int
	title  string
	// opStatus: the status to go back to, and the one being left.
	toStatus, fromStatus model.Status
	// opProgress: the page to go back to, and the one being left.
	toPage, fromPage int
}

// ── Toasts ─────────────────────────────────────────────────────────────────

// toastLevel is how a toast is drawn: the colour, and the glyph for the
// terminals that have none.
type toastLevel int

const (
	toastInfo toastLevel = iota
	toastWarn
	toastError
)

// toast is the status bar's message. It expires on its own tick, stamped
// with seq so the tick of a toast that has since been replaced is ignored.
type toast struct {
	level toastLevel
	text  string
	seq   int
}

type toastExpiredMsg struct{ seq int }

const (
	toastTTL      = 5 * time.Second
	toastErrorTTL = 8 * time.Second
)

type backgroundCheckMsg struct{}

type timerTickMsg time.Time

type localDataLoadedMsg struct {
	readingStats   *model.ReadingStats
	recentSessions []model.ReadingSession
	recentSearches []string
	timerState     *model.TimerState
	timerBook      *model.Book
	err            error
}

type timerOpDoneMsg struct {
	info    string
	err     error
	session *model.ReadingSession
}

// ── Dashboard Model ────────────────────────────────────────────────────────

type dashboardModel struct {
	app *app.App
	// ctx is cancelled when the program exits so in-flight API calls abort
	// instead of outliving the store.
	ctx     context.Context
	version string

	mode    viewMode
	section focusSection
	loaded  bool
	// inflight counts the commands that are running. A single boolean meant
	// the first completion cleared the flag for every other operation, so a
	// fast local load stopped the spinner mid-sync. Every command started
	// through beginLoading must produce exactly one message whose handler
	// calls endLoading.
	inflight int

	// spinning reports whether the spinner tick loop is armed. It only runs
	// while work is in flight, so an idle dashboard does not re-render a
	// dozen times a second.
	spinning bool
	// reconciling marks the background reconcile load; only its success
	// clears dirty.
	reconciling bool

	width  int
	height int

	readingList       list.Model
	okuList           list.Model
	searchList        list.Model
	searchInput       textinput.Model
	pageInput         textinput.Model
	reviewRatingInput textinput.Model
	reviewTextInput   textarea.Model
	spin              spinner.Model
	help              help.Model

	readingBooks []model.UserBook
	okuBooks     []model.UserBook
	searchBooks  []model.SearchResult

	pendingBookID int
	// The page prompt shows which book it is about and where that book
	// stands, so the input can keep the format hint as its placeholder.
	pageBookTitle   string
	pageCurrentPage int
	pageTotalPages  int
	// pageSubmitting marks that the page modal is waiting for its own update,
	// so a progress result started before it opened cannot close it.
	pageSubmitting   bool
	reviewBook       *model.UserBook
	reviewFocus      dashboardReviewFocus
	reviewSubmitting bool
	// reviewSeq identifies the modal session a save belongs to, so the result
	// of a cancelled (or reopened) modal is ignored.
	reviewSeq int
	// reviewErr is the review modal's own error, rendered inside the overlay
	// because the status bar sits behind it.
	reviewErr string

	dirty          bool
	lastMutationAt time.Time

	lastQuery      string
	lastSearchMode model.SearchMode

	// toast is the status bar's message, and undo what U would reverse
	// while it is up. Both are dropped when the toast expires.
	toast toast
	undo  *undoAction

	searchLoading      bool
	searchLoadingQuery string
	searchSeq          int
	searchMode         searchInputMode
	searchSub          searchSubFocus
	searchQueryMode    model.SearchMode
	recentSearches     []string
	density            outputDensity

	// confirm guards the library keys that are hard to walk back from, and
	// confirmCmd is the operation it is holding.
	confirm    confirmState
	confirmCmd tea.Cmd

	showHelp bool
	// helpViewport scrolls the help modal, whose body is taller than a short
	// terminal.
	helpViewport viewport.Model

	// Local data for stats/timer sections.
	timerState     *model.TimerState
	readingStats   *model.ReadingStats
	weeklyStats    model.WeeklyStats
	recentSessions []model.ReadingSession
	localLoaded    bool
	statsScroll    int
	// timerBook is the running timer's book, resolved when local data loads so
	// that View never queries the store.
	timerBook *model.Book
	// timerTicking reports whether the one-second tick loop is armed; it only
	// runs while a timer is actually running.
	timerTicking bool

	timerSelecting bool
	timerSelectIdx int
}

func newDashboardModel(ctx context.Context, a *app.App) dashboardModel {
	if ctx == nil {
		ctx = context.Background()
	}

	delegate := newListDelegate(0)

	// The section card already prints the name and the count, and the panels
	// are only a handful of rows tall, so a list spends none of them on its
	// own title bar or on pagination dots. Filtering stays enabled (SetItems
	// reapplies an active filter) but its title-bar row is not drawn.
	newList := func() list.Model {
		l := list.New(nil, delegate, 40, 12)
		l.SetShowTitle(false)
		l.SetShowFilter(false)
		l.SetShowStatusBar(false)
		l.SetShowPagination(false)
		l.SetFilteringEnabled(true)
		l.SetShowHelp(false)
		l.DisableQuitKeybindings()
		return l
	}

	searchL := newList()
	// The search results panel has no card label, so it prints this title as
	// its own one-line header.
	searchL.Title = model.SearchModeBook.Label() + " Results"

	searchIn := textinput.New()
	searchIn.Placeholder = "Search books..."
	searchIn.CharLimit = 120
	searchIn.Prompt = "/ "
	searchIn.PromptStyle = lipgloss.NewStyle().Foreground(th.accent).Bold(true)
	searchIn.TextStyle = lipgloss.NewStyle().Foreground(th.text)
	// Suggestions are the user's own search history, loaded with the rest of
	// the local data; there are none until they have searched for something.
	searchIn.ShowSuggestions = true

	pageIn := textinput.New()
	pageIn.Placeholder = "370 or +10 or -5"
	pageIn.CharLimit = 32
	pageIn.Prompt = "› "
	pageIn.PromptStyle = lipgloss.NewStyle().Foreground(th.accent).Bold(true)
	pageIn.TextStyle = lipgloss.NewStyle().Foreground(th.text)

	// The review fields sit inside a modal, so they carry its background: a
	// style that only sets a foreground would leave the field on the
	// terminal's own colour.
	reviewRatingIn := textinput.New()
	reviewRatingIn.Placeholder = "4.5"
	reviewRatingIn.CharLimit = 4
	reviewRatingIn.Prompt = reviewFieldFocused + "Rating: "
	reviewRatingIn.PromptStyle = modalKeyStyle
	reviewRatingIn.TextStyle = modalValueStyle
	reviewRatingIn.PlaceholderStyle = modalDimStyle

	reviewTextIn := textarea.New()
	reviewTextIn.Placeholder = "Write your review..."
	reviewTextIn.SetWidth(60)
	reviewTextIn.SetHeight(8)
	reviewTextIn.ShowLineNumbers = false
	for _, st := range []*textarea.Style{&reviewTextIn.FocusedStyle, &reviewTextIn.BlurredStyle} {
		st.Base = modalBgStyle
		st.Text = modalValueStyle
		st.Placeholder = modalDimStyle
		st.Prompt = modalKeyStyle
		st.CursorLine = modalBgStyle
		st.EndOfBuffer = modalBgStyle
	}

	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(th.accent)

	hlp := help.New()
	hlp.ShortSeparator = " · "
	hlp.Styles.ShortKey = keyStyle
	hlp.Styles.ShortDesc = descStyle
	hlp.Styles.ShortSeparator = dimStyleTUI
	hlp.Styles.Ellipsis = dimStyleTUI

	return dashboardModel{
		app:               a,
		ctx:               ctx,
		mode:              modeLibrary,
		section:           sectionReading,
		searchMode:        searchModeNormal,
		searchSub:         searchSubInput,
		searchQueryMode:   model.SearchModeBook,
		density:           currentOutputDensity(),
		readingList:       newList(),
		okuList:           newList(),
		searchList:        searchL,
		searchInput:       searchIn,
		pageInput:         pageIn,
		reviewRatingInput: reviewRatingIn,
		reviewTextInput:   reviewTextIn,
		spin:              s,
		help:              hlp,
		// Init starts the cached-library and local-data loads, so two
		// commands are already in flight.
		inflight: 2,
		spinning: true,
	}
}

// ── Init ───────────────────────────────────────────────────────────────────

func (m dashboardModel) Init() tea.Cmd {
	return tea.Batch(
		m.spin.Tick,
		loadCachedLibraryCmd(m.app),
		loadLocalDataCmd(m.app),
		backgroundCheckCmd(),
	)
}

// ── Update ─────────────────────────────────────────────────────────────────

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, isKey := msg.(tea.KeyMsg)
	if !isKey {
		// Async results, ticks and resizes are handled once for every mode so
		// an open modal never swallows them.
		return m.updateCommon(msg)
	}

	// The confirmation takes the keys whatever is behind it. Async results
	// still reach updateCommon above, so a reload landing mid-question is
	// applied as usual.
	if m.confirm.Active {
		return m.updateConfirmMode(key)
	}

	switch m.mode {
	case modeUpdatePage:
		return m.updatePageMode(key)
	case modeReviewRating:
		return m.updateReviewRatingMode(key)
	default:
		return m.updateLibraryMode(key)
	}
}

// updateCommon handles every message that is not a key press, whatever mode
// the dashboard is in.
func (m dashboardModel) updateCommon(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if !m.isLoading() && !m.searchLoading {

			// Nothing in flight: let the tick loop stop.
			m.spinning = false
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case timerTickMsg:
		if m.timerState == nil {
			// Nothing to animate: let the tick loop stop.
			m.timerTicking = false
			return m, nil
		}
		// Just triggers a re-render for the timer display.
		return m, timerTickCmd()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		m.reviewTextInput.SetWidth(max(40, m.width/2))
		return m, nil

	case libraryLoadedMsg:
		return m.applyLibraryLoaded(msg)

	case localDataLoadedMsg:
		m.endLoading()
		m.localLoaded = true

		if msg.err != nil {
			return m, m.showToast(toastError, msg.err.Error())
		}
		m.readingStats = msg.readingStats
		if msg.readingStats != nil {
			m.weeklyStats = msg.readingStats.Weekly
		}
		m.recentSessions = msg.recentSessions
		m.mergeRecentSearches(msg.recentSearches)
		m.updateSearchSuggestions()
		m.timerState = msg.timerState
		m.timerBook = msg.timerBook
		cmd := m.startTimerTick()
		return m, cmd

	case timerOpDoneMsg:
		m.endLoading()
		m.timerSelecting = false

		var toastCmd tea.Cmd
		if msg.err != nil {
			toastCmd = m.showToast(toastError, msg.err.Error())
		} else {
			toastCmd = m.showToast(toastInfo, msg.info)
		}
		// Reload local data after timer operations. The toast tick is not
		// work, so it stays out of the in-flight count.
		reload := m.beginLoading(loadLocalDataCmd(m.app))
		return m, tea.Batch(reload, toastCmd)

	case toastExpiredMsg:
		// Only the current toast's own tick clears it; a tick left over from
		// an earlier toast must not take a newer one down with it.
		if msg.seq == m.toast.seq {
			m.toast = toast{seq: m.toast.seq}
			m.undo = nil
		}
		return m, nil

	case searchLoadedMsg:
		return m.applySearchLoaded(msg)

	case backgroundCheckMsg:
		if m.dirty && !m.isLoading() && !m.reconciling && time.Since(m.lastMutationAt) >= backgroundSyncWindow {

			m.reconciling = true
			cmd := m.beginLoading(reconcileLibraryCmd(m.ctx, m.app))
			return m, tea.Batch(cmd, backgroundCheckCmd())
		}
		return m, backgroundCheckCmd()

	case opDoneMsg:
		return m.applyOpDone(msg)
	}

	// Anything else (cursor blinks, list filter updates) goes to whatever has
	// focus right now.
	var cmd tea.Cmd
	switch m.mode {
	case modeUpdatePage:
		m.pageInput, cmd = m.pageInput.Update(msg)
		return m, cmd
	case modeReviewRating:
		if m.reviewFocus == dashboardReviewFocusRating {
			m.reviewRatingInput, cmd = m.reviewRatingInput.Update(msg)
		} else {
			m.reviewTextInput, cmd = m.reviewTextInput.Update(msg)
		}
		return m, cmd
	}

	switch m.section {
	case sectionReading:
		m.readingList, cmd = m.readingList.Update(msg)
	case sectionOku:
		m.okuList, cmd = m.okuList.Update(msg)
	case sectionSearch:
		if m.searchSub == searchSubResults {
			m.searchList, cmd = m.searchList.Update(msg)
		}
	}
	return m, cmd
}

func (m dashboardModel) applyLibraryLoaded(msg libraryLoadedMsg) (tea.Model, tea.Cmd) {
	m.endLoading()
	m.loaded = true
	if msg.reconcile {
		m.reconciling = false
	}
	if msg.err != nil {
		// dirty stays set: the local mutations are still unreconciled.
		return m, m.showToast(toastError, msg.err.Error())
	}
	m.readingBooks = msg.reading
	m.okuBooks = msg.oku
	if m.timerSelectIdx >= len(m.readingBooks) {
		m.timerSelectIdx = max(0, len(m.readingBooks)-1)
	}
	cmd := m.refreshListItems()
	m.updateSearchSuggestions()
	if msg.reconcile {
		// The pending local mutations are now reflected by the server data.
		m.dirty = false
	}
	if msg.needsRefresh {
		refresh := m.beginLoading(loadLibraryCmd(m.ctx, m.app, true))
		return m, tea.Batch(cmd, refresh)
	}
	return m, cmd
}

func (m dashboardModel) applySearchLoaded(msg searchLoadedMsg) (tea.Model, tea.Cmd) {
	// The command has finished either way, so its slot is released first.
	m.endLoading()
	if msg.seq != m.searchSeq {
		// A newer search superseded this one.
		return m, nil
	}
	m.searchLoading = false

	m.searchLoadingQuery = ""
	if msg.err != nil {
		// The previous results are still on screen, so the header keeps
		// naming them - including how many there are.
		m.refreshSearchTitle()
		return m, m.showToast(toastError, msg.err.Error())
	}
	// Labels come from the mode the results were fetched with; the user may
	// have switched modes since.
	label := msg.mode.Label()
	m.lastQuery = msg.query
	m.lastSearchMode = msg.mode
	m.searchBooks = msg.results
	cmd := m.refreshSearchResultItems()
	m.refreshSearchTitle()
	if len(msg.results) > 0 && m.section == sectionSearch {
		// Only take focus if the user is still in the search section.
		m.searchSub = searchSubResults
		m.enterSearchNormalMode()
	}
	var toastCmd tea.Cmd
	if len(msg.results) == 0 {
		toastCmd = m.showToast(toastInfo, fmt.Sprintf("%s mode: no results for %q", strings.ToLower(label), msg.query))
	} else {
		toastCmd = m.showToast(toastInfo, fmt.Sprintf("%s mode: loaded %d results", strings.ToLower(label), len(msg.results)))
	}
	saveCmd := m.addRecentSearchQuery(msg.query)
	m.updateSearchSuggestions()
	return m, tea.Batch(cmd, saveCmd, toastCmd)
}

// applyOpDone applies the result of a mutating operation. Results other than
// the review modal's own save leave the modal, and its draft, untouched.
func (m dashboardModel) applyOpDone(msg opDoneMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeReviewRating && msg.op == opReview &&
		m.reviewSubmitting && msg.seq == m.reviewSeq {
		return m.applyReviewSaveDone(msg)
	}

	m.endLoading()

	toastCmd := m.toastFor(msg)
	if msg.err == nil && msg.markDirty {
		m.dirty = true
		m.lastMutationAt = time.Now()
	}
	if m.mode == modeUpdatePage && m.pageSubmitting && msg.op == opProgress {
		m.closePageModal()
	}
	if msg.reload {
		reload := m.beginLoading(loadLibraryCmd(m.ctx, m.app, false), loadLocalDataCmd(m.app))
		return m, tea.Batch(reload, toastCmd)
	}
	return m, toastCmd
}

// applyReviewSaveDone keeps the review modal open until its own save
// succeeds, so a failed save never throws away what was typed.
func (m dashboardModel) applyReviewSaveDone(msg opDoneMsg) (tea.Model, tea.Cmd) {
	m.endLoading()
	m.reviewSubmitting = false

	if msg.err != nil {
		// Shown in the overlay; the rating and review text are preserved.
		m.reviewErr = msg.err.Error()
		return m, nil
	}
	toastCmd := m.showToast(toastInfo, msg.info)
	if msg.markDirty {
		m.dirty = true
		m.lastMutationAt = time.Now()
	}
	m.closeReviewRatingModal()
	if msg.reload {
		reload := m.beginLoading(loadLibraryCmd(m.ctx, m.app, false), loadLocalDataCmd(m.app))
		return m, tea.Batch(reload, toastCmd)
	}
	return m, toastCmd
}

// showToast replaces the status bar's message and returns the tick that
// clears it. The previous toast's undo goes with it: the message that named
// it is gone.
func (m *dashboardModel) showToast(level toastLevel, text string) tea.Cmd {
	m.toast = toast{level: level, text: text, seq: m.toast.seq + 1}
	m.undo = nil

	ttl := toastTTL
	if level == toastError {
		ttl = toastErrorTTL
	}
	seq := m.toast.seq
	return tea.Tick(ttl, func(time.Time) tea.Msg {
		return toastExpiredMsg{seq: seq}
	})
}

// showUndoToast is showToast for a change that can be reversed while the
// toast is up.
func (m *dashboardModel) showUndoToast(text string, undo undoAction) tea.Cmd {
	cmd := m.showToast(toastInfo, text)
	m.undo = &undo
	return cmd
}

// toastFor reports an operation's result: its error, or its info with an
// undo when the result says how to reverse it.
func (m *dashboardModel) toastFor(msg opDoneMsg) tea.Cmd {
	if msg.err != nil {
		return m.showToast(toastError, msg.err.Error())
	}
	switch {
	case msg.op == opStatus && msg.bookID > 0 && msg.prevStatus != 0 && msg.prevStatus != msg.newStatus:
		return m.showUndoToast(
			fmt.Sprintf("Moved '%s' to %s", msg.title, msg.newStatus.Label()),
			undoAction{op: opStatus, bookID: msg.bookID, title: msg.title,
				toStatus: msg.prevStatus, fromStatus: msg.newStatus},
		)
	case msg.op == opProgress && msg.bookID > 0 && msg.prevPage != msg.newPage:
		return m.showUndoToast(
			fmt.Sprintf("Page %d", msg.newPage),
			undoAction{op: opProgress, bookID: msg.bookID, title: msg.title,
				toPage: msg.prevPage, fromPage: msg.newPage},
		)
	}
	if msg.info == "" {
		return nil
	}
	return m.showToast(toastInfo, msg.info)
}

// runUndo reverses the change the current toast reports, if there is one.
func (m dashboardModel) runUndo() (tea.Model, tea.Cmd) {
	u := m.undo
	if u == nil {
		return m, nil
	}
	m.undo = nil
	switch u.op {
	case opStatus:
		return m.startOp(changeStatusCmd(m.ctx, m.app, u.bookID, u.title, u.fromStatus, u.toStatus))
	case opProgress:
		return m.startOp(updateProgressCmd(m.ctx, m.app, u.bookID, u.title, u.fromPage, strconv.Itoa(u.toPage)))
	}
	return m, nil
}

// startOp marks work as in flight and returns the model together with cmds and
// the spinner tick.
func (m dashboardModel) startOp(cmds ...tea.Cmd) (tea.Model, tea.Cmd) {
	cmd := m.beginLoading(cmds...)
	return m, cmd
}

// beginLoading counts one in-flight command per cmd and (re)starts the spinner
// tick loop. Each cmd must produce exactly one message whose handler calls
// endLoading.
func (m *dashboardModel) beginLoading(cmds ...tea.Cmd) tea.Cmd {
	for _, cmd := range cmds {
		if cmd != nil {
			m.inflight++
		}
	}
	return tea.Batch(append(cmds, m.startSpinner())...)
}

// endLoading records that one in-flight command has produced its result.
func (m *dashboardModel) endLoading() {
	if m.inflight > 0 {
		m.inflight--
	}
}

// isLoading reports whether any command is still running.
func (m dashboardModel) isLoading() bool {
	return m.inflight > 0
}

// startSpinner returns the spinner tick only when work is in flight and no
// loop is armed, so overlapping operations never run two of them at once.
func (m *dashboardModel) startSpinner() tea.Cmd {
	if m.spinning || (m.inflight == 0 && !m.searchLoading) {
		return nil
	}
	m.spinning = true
	return m.spin.Tick
}

// startTimerTick arms the one-second tick that refreshes the elapsed time,
// but only while a timer runs and no tick loop is armed yet.
func (m *dashboardModel) startTimerTick() tea.Cmd {
	if m.timerTicking || m.timerState == nil {
		return nil
	}
	m.timerTicking = true
	return timerTickCmd()
}

func (m dashboardModel) updateLibraryMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Help modal intercepts all keys.
	if m.showHelp {
		k := m.activeKeys()
		switch {
		case key.Matches(msg, k.Help, k.Back):
			m.showHelp = false
		case key.Matches(msg, k.Quit, k.ForceQuit):
			return m, tea.Quit
		case key.Matches(msg, k.Down):
			m.helpViewport.LineDown(1)
		case key.Matches(msg, k.Up):
			m.helpViewport.LineUp(1)
		case key.Matches(msg, k.HalfPageDown):
			m.helpViewport.HalfViewDown()
		case key.Matches(msg, k.HalfPageUp):
			m.helpViewport.HalfViewUp()
		case key.Matches(msg, k.ScrollTop):
			m.helpViewport.GotoTop()
		case key.Matches(msg, k.ScrollBottom):
			m.helpViewport.GotoBottom()
		}
		return m, nil
	}

	// Undo is the one key every section shares while its toast is up.
	if key.Matches(msg, m.activeKeys().Undo) {
		return m.runUndo()
	}

	// Route to section-specific handlers.
	switch m.section {
	case sectionSearch:
		return m.handleSearchKeys(msg)
	case sectionReading, sectionOku:
		return m.handleLibraryKeys(msg)
	case sectionStats:
		return m.handleStatsKeys(msg)
	case sectionTimer:
		return m.handleTimerKeys(msg)
	default:
		return m.handleGenericKeys(msg)
	}
}

// ── Section-specific key handlers ──────────────────────────────────────────

func (m dashboardModel) handleGenericKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	switch {
	case key.Matches(msg, k.Quit, k.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.openHelp()
		return m, nil
	case key.Matches(msg, k.Down, k.NextSection):
		m.nextSection()
		return m, nil
	case key.Matches(msg, k.Up, k.PrevSection):
		m.prevSection()
		return m, nil
	case key.Matches(msg, k.Search):
		m.focusSearchInput()
		return m, nil
	}
	return m, nil
}

// focusSearchInput jumps to the search card in insert mode, keeping whatever
// query was already typed.
func (m *dashboardModel) focusSearchInput() {
	m.setSection(sectionSearch)
	m.searchSub = searchSubInput
	m.enterSearchInsertMode()
	m.searchInput.CursorEnd()
	m.updateSearchSuggestions()
}

func (m dashboardModel) handleStatsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	switch {
	case key.Matches(msg, k.Down):
		content := m.statsView(m.rightPanelContentWidth())
		_, m.statsScroll = clipLines(content, m.statsScroll+1, m.rightPanelContentHeight())
		return m, nil
	case key.Matches(msg, k.Up):
		if m.statsScroll > 0 {
			m.statsScroll--
		}
		return m, nil
	case key.Matches(msg, k.ScrollTop):
		m.statsScroll = 0
		return m, nil
	case key.Matches(msg, k.Refresh):
		return m.startOp(loadLocalDataCmd(m.app))
	case key.Matches(msg, k.Sync):
		return m.startOp(syncAllAndReloadCmd(m.ctx, m.app))
	case key.Matches(msg, k.NextSection):
		m.statsScroll = 0
		m.nextSection()
		return m, nil
	case key.Matches(msg, k.PrevSection):
		m.statsScroll = 0
		m.prevSection()
		return m, nil
	}
	return m.handleGenericKeys(msg)
}

func (m dashboardModel) handleLibraryKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	switch {
	case key.Matches(msg, k.Quit, k.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.openHelp()
		return m, nil
	case key.Matches(msg, k.NextSection):
		m.nextSection()
		return m, nil
	case key.Matches(msg, k.PrevSection):
		m.prevSection()
		return m, nil
	case key.Matches(msg, k.Up, k.Down):
		// The focused list moves its own cursor.
		var cmd tea.Cmd
		if m.section == sectionReading {
			m.readingList, cmd = m.readingList.Update(msg)
		} else {
			m.okuList, cmd = m.okuList.Update(msg)
		}
		return m, cmd
	case key.Matches(msg, k.Search):
		m.focusSearchInput()
		return m, nil
	case key.Matches(msg, k.Refresh):
		return m.startOp(loadLibraryCmd(m.ctx, m.app, true))
	case key.Matches(msg, k.Sync):
		return m.startOp(syncAllAndReloadCmd(m.ctx, m.app))
	case key.Matches(msg, k.Density):
		cmd := m.cycleDensity()
		return m, cmd
	case key.Matches(msg, k.Details):
		// Enter used to move the book to another shelf, so a stray keypress
		// silently rewrote the library. It now only brings the selection into
		// the detail pane; g/w/f/d still change the status.
		b := m.selectedLibraryBook()
		if b == nil {
			return m, m.showToast(toastError, "no book selected")
		}
		return m, m.showToast(toastInfo, b.Book.Title)
	case key.Matches(msg, k.ProgressUp):
		return m.quickProgress(+10)
	case key.Matches(msg, k.ProgressDown):
		return m.quickProgress(-10)
	case key.Matches(msg, k.Update):
		if b := m.selectedLibraryBook(); b != nil {
			m.openPageModal(*b)
			return m, nil
		}
	case key.Matches(msg, k.Rate):
		if b := m.selectedLibraryBook(); b != nil {
			m.openReviewRatingModal(*b)
			return m, nil
		}
	case key.Matches(msg, k.SetReading):
		return m.changeSelectedLibraryStatus(model.StatusCurrentlyReading)
	case key.Matches(msg, k.SetWant):
		return m.changeSelectedLibraryStatus(model.StatusWantToRead)
	case key.Matches(msg, k.SetFinished):
		return m.changeSelectedLibraryStatus(model.StatusRead)
	case key.Matches(msg, k.SetDNF):
		return m.confirmStatusChange(model.StatusDidNotFinish)
	case key.Matches(msg, k.SetIgnored):
		return m.confirmStatusChange(model.StatusIgnored)
	case key.Matches(msg, k.Timer):
		return m.toggleTimerForSelection()
	}
	return m, nil
}

// toggleTimerForSelection starts a reading timer for the highlighted book, or
// stops the one that is running. Only the Reading list holds books a timer can
// track, so elsewhere it says where to press it.
func (m dashboardModel) toggleTimerForSelection() (tea.Model, tea.Cmd) {
	if m.isLoading() {
		// timerState only catches up when the load lands, so two quick presses
		// would otherwise start two sessions.
		return m, m.showToast(toastWarn, inFlightNotice)
	}
	if m.timerState != nil {
		return m.startOp(stopTimerCmd(m.app))
	}
	if m.section != sectionReading {
		return m, m.showToast(toastWarn, "Timers track a book you are reading — press t in the Reading list")
	}
	b := m.selectedLibraryBook()
	if b == nil {
		return m, m.showToast(toastError, "no book selected")
	}
	return m.startOp(startTimerForBookCmd(m.app, b.Book.ID))
}

// inFlightNotice is the answer to a mutation pressed while one is running.
const inFlightNotice = "Please wait — an update is still in flight"

// confirmStatusChange asks first. Ignoring a book takes it out of the library
// and a DNF closes the read, and neither can be undone from the dashboard, so
// they should not happen because a finger slipped one key.
func (m dashboardModel) confirmStatusChange(status model.Status) (tea.Model, tea.Cmd) {
	b := m.selectedLibraryBook()
	if b == nil {
		return m, m.showToast(toastError, "no book selected")
	}
	m.confirm = newConfirmState(fmt.Sprintf("Mark '%s' as %s?", b.Book.Title, status.Label()))
	m.confirmCmd = changeStatusCmd(m.ctx, m.app, b.Book.ID, b.Book.Title, b.StatusID, status)
	return m, nil
}

// updateConfirmMode answers the confirmation: y (or Enter on Confirm) runs the
// operation it is holding, n and Esc drop it.
func (m dashboardModel) updateConfirmMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	if key.Matches(msg, k.ForceQuit) {
		return m, tea.Quit
	}

	confirmed, handled := m.confirm.handleKey(msg, k)
	if !handled || m.confirm.Active {
		// An unknown key, or the cursor only moved between the buttons.
		return m, nil
	}

	pending := m.confirmCmd
	m.confirm = confirmState{}
	m.confirmCmd = nil
	if !confirmed {
		return m, m.showToast(toastInfo, "Cancelled")
	}
	return m.startOp(pending)
}

func (m dashboardModel) handleSearchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	if m.searchSub == searchSubInput {
		if m.searchMode == searchModeNormal {
			switch {
			case key.Matches(msg, k.ForceQuit):
				return m, tea.Quit
			case key.Matches(msg, k.Help):
				m.openHelp()
				return m, nil
			case key.Matches(msg, k.SearchInsert):
				m.enterSearchInsertMode()
				return m, nil
			case key.Matches(msg, k.SearchAppend):
				m.enterSearchInsertMode()
				m.searchInput.CursorEnd()
				return m, nil
			case key.Matches(msg, k.SearchMode):
				return m, m.setSearchQueryMode(m.searchQueryMode.Next())
			case key.Matches(msg, k.SearchModeBook):
				return m, m.setSearchQueryMode(model.SearchModeBook)
			case key.Matches(msg, k.SearchModeAuthor):
				return m, m.setSearchQueryMode(model.SearchModeAuthor)
			case key.Matches(msg, k.SearchModeGenre):
				return m, m.setSearchQueryMode(model.SearchModeGenre)
			case key.Matches(msg, k.Density):
				cmd := m.cycleDensity()
				return m, cmd
			case key.Matches(msg, k.SearchSubmit):
				cmd := m.submitSearch()
				return m, cmd
			case key.Matches(msg, k.NextSection):
				m.nextSection()
				m.searchInput.Blur()
				return m, nil
			case key.Matches(msg, k.Back, k.PrevSection, k.Up):
				m.prevSection()
				m.searchInput.Blur()
				return m, nil
			case key.Matches(msg, k.Down):
				if m.hasSearchResults() {
					m.searchSub = searchSubResults
				} else {
					m.nextSection()
				}
				return m, nil
			}
			return m, nil
		}

		// Insert mode.
		switch {
		case key.Matches(msg, k.ForceQuit):
			return m, tea.Quit
		case key.Matches(msg, k.SearchSubmit):
			cmd := m.submitSearch()
			return m, cmd
		case key.Matches(msg, k.Back):
			m.enterSearchNormalMode()
			return m, nil
		}

		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	// searchSubResults
	switch {
	case key.Matches(msg, k.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.openHelp()
		return m, nil
	case key.Matches(msg, k.SearchBack):
		// Checked before PrevSection, which shares h and left with it.
		m.searchSub = searchSubInput
		m.enterSearchNormalMode()
		return m, nil
	case key.Matches(msg, k.NextSection):
		m.nextSection()
		return m, nil
	case key.Matches(msg, k.PrevSection):
		m.prevSection()
		return m, nil
	case key.Matches(msg, k.AddReading):
		if r := m.selectedSearchResult(); r != nil {
			return m.startOp(addFromSearchCmd(m.ctx, m.app, r.ID, model.StatusCurrentlyReading))
		}
	case key.Matches(msg, k.SetReading):
		return m.changeSelectedSearchStatus(model.StatusCurrentlyReading)
	case key.Matches(msg, k.SetWant):
		return m.changeSelectedSearchStatus(model.StatusWantToRead)
	case key.Matches(msg, k.SetFinished):
		return m.changeSelectedSearchStatus(model.StatusRead)
	case key.Matches(msg, k.SetDNF):
		return m.changeSelectedSearchStatus(model.StatusDidNotFinish)
	case key.Matches(msg, k.Density):
		cmd := m.cycleDensity()
		return m, cmd
	}

	var cmd tea.Cmd
	m.searchList, cmd = m.searchList.Update(msg)
	return m, cmd
}

func (m dashboardModel) handleTimerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	if m.timerSelecting && m.timerState == nil {
		switch {
		case key.Matches(msg, k.Quit, k.ForceQuit):
			return m, tea.Quit
		case key.Matches(msg, k.Help):
			m.openHelp()
			return m, nil
		case key.Matches(msg, k.Back):
			m.timerSelecting = false
			return m, m.showToast(toastInfo, "Timer start cancelled")
		case key.Matches(msg, k.Down):
			if m.timerSelectIdx < len(m.readingBooks)-1 {
				m.timerSelectIdx++
			}
			return m, nil
		case key.Matches(msg, k.Up):
			if m.timerSelectIdx > 0 {
				m.timerSelectIdx--
			}
			return m, nil
		case key.Matches(msg, k.Select):
			if len(m.readingBooks) == 0 {
				m.timerSelecting = false
				return m, m.showToast(toastError, "no currently reading books available")
			}
			// Background sync can shrink readingBooks while the picker is open.
			if m.timerSelectIdx >= len(m.readingBooks) {
				m.timerSelectIdx = len(m.readingBooks) - 1
			}
			selected := m.readingBooks[m.timerSelectIdx]
			m.timerSelecting = false
			return m.startOp(startTimerForBookCmd(m.app, selected.Book.ID))
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, k.Quit, k.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.openHelp()
		return m, nil
	case key.Matches(msg, k.Down, k.NextSection):
		m.nextSection()
		return m, nil
	case key.Matches(msg, k.Up, k.PrevSection):
		m.prevSection()
		return m, nil
	case key.Matches(msg, k.Search):
		m.focusSearchInput()
		return m, nil
	case key.Matches(msg, k.Timer):
		if m.timerState != nil {
			// Same key, same meaning as in the library: t toggles.
			return m.startOp(stopTimerCmd(m.app))
		}
		if len(m.readingBooks) == 0 {
			return m, m.showToast(toastError, "no currently reading books available — add a book to Reading first")
		}

		m.timerSelecting = true
		m.timerSelectIdx = 0
		if selected := m.selectedLibraryBook(); selected != nil {
			for i, b := range m.readingBooks {
				if b.Book.ID == selected.Book.ID {
					m.timerSelectIdx = i
					break
				}
			}
		}
		return m, m.showToast(toastInfo, "Select a book and press Enter to start timer")
	case key.Matches(msg, k.TimerStop):
		// Only enabled while a timer runs.
		return m.startOp(stopTimerCmd(m.app))
	}
	return m, nil
}

// ── Page update mode ───────────────────────────────────────────────────────

func (m dashboardModel) updatePageMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	switch {
	case key.Matches(msg, k.Back):
		m.closePageModal()
		return m, nil
	case key.Matches(msg, k.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, k.Select):
		raw := strings.TrimSpace(m.pageInput.Value())
		if raw == "" {
			return m, m.showToast(toastError, "page value cannot be empty")
		}
		if m.isLoading() {
			return m, m.showToast(toastWarn, inFlightNotice)
		}
		m.pageSubmitting = true
		return m.startOp(updateProgressCmd(m.ctx, m.app, m.pendingBookID, m.pageBookTitle, m.pageCurrentPage, raw))
	}

	var cmd tea.Cmd
	m.pageInput, cmd = m.pageInput.Update(msg)
	return m, cmd
}

// openPageModal opens the page prompt for a book. The input starts empty, so
// an accidental Enter cannot rewrite the progress, and keeps its format hint
// as the placeholder: the title and the current page get lines of their own.
func (m *dashboardModel) openPageModal(b model.UserBook) {
	m.mode = modeUpdatePage
	m.pendingBookID = b.Book.ID
	m.pageBookTitle = b.Book.Title
	m.pageCurrentPage = b.CurrentPage
	if len(b.UserBookReads) > 0 {
		m.pageCurrentPage = b.UserBookReads[0].ProgressPages
	}
	m.pageTotalPages = b.Book.Pages
	m.pageInput.SetValue("")
	m.pageInput.Focus()
	// The prompt is taller than the help bar it replaces.
	m.resize()
}

func (m *dashboardModel) closePageModal() {
	m.mode = modeLibrary
	m.pageSubmitting = false
	m.pageBookTitle = ""
	m.pageCurrentPage = 0
	m.pageTotalPages = 0
	m.pageInput.Blur()
	m.pageInput.SetValue("")
	m.resize()
}

// pagePrompt renders the page-update prompt under the layout. It is always
// pagePromptRows tall, which is what the layout height is computed against.
func (m dashboardModel) pagePrompt() string {
	current := fmt.Sprintf("current: page %d", m.pageCurrentPage)
	if m.pageTotalPages > 0 {
		current = fmt.Sprintf("current: %d/%d", m.pageCurrentPage, m.pageTotalPages)
	}
	return "\n" + strings.Join([]string{
		" " + keyStyle.Render("Update page") + "  " + valueStyle.Render(m.pageBookTitle),
		" " + dimStyleTUI.Render(current),
		" " + m.pageInput.View() + dimStyleTUI.Render("   Enter save · Esc cancel"),
	}, "\n")
}

// quickProgress applies a relative page update. UpdateProgress is
// read-modify-write, so firing a second one while the first is in flight
// would silently lose an update.
func (m dashboardModel) quickProgress(delta int) (tea.Model, tea.Cmd) {
	b := m.selectedLibraryBook()
	if b == nil {
		return m, nil
	}
	if m.isLoading() {
		return m, m.showToast(toastWarn, inFlightNotice)
	}
	return m.startOp(quickProgressCmd(m.ctx, m.app, b.Book.ID, b.Book.Title, currentPage(*b), delta))
}

// currentPage is where a book stands: the open read's progress when there
// is one, the book's own page otherwise.
func currentPage(b model.UserBook) int {
	if len(b.UserBookReads) > 0 {
		return b.UserBookReads[0].ProgressPages
	}
	return b.CurrentPage
}

// ── Review/rating mode ─────────────────────────────────────────────────────

func (m dashboardModel) updateReviewRatingMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	if m.reviewSubmitting {
		// The fields are read-only until the save reports back; cancelling
		// bumps reviewSeq, so the pending result is ignored.
		switch {
		case key.Matches(msg, k.ForceQuit):
			return m, tea.Quit
		case key.Matches(msg, k.Back):
			m.closeReviewRatingModal()
			return m, m.showToast(toastInfo, "Review update cancelled")
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, k.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, k.Back):
		m.closeReviewRatingModal()
		return m, m.showToast(toastInfo, "Review update cancelled")
	case key.Matches(msg, k.ReviewNextField):
		if m.reviewFocus == dashboardReviewFocusRating {
			m.focusReviewTextField()
		} else {
			m.focusReviewRatingField()
		}
		return m, nil
	case key.Matches(msg, k.ReviewPrevField):
		if m.reviewFocus == dashboardReviewFocusText {
			m.focusReviewRatingField()
		} else {
			m.focusReviewTextField()
		}
		return m, nil
	case key.Matches(msg, k.ReviewSave):
		if m.reviewBook == nil {
			return m, nil
		}
		rating, err := parseReviewRatingInput(m.reviewRatingInput.Value())
		if err != nil {
			m.reviewErr = err.Error()
			return m, nil
		}
		review := m.reviewTextInput.Value()
		m.reviewErr = ""
		toastCmd := m.showToast(toastInfo, reviewSavePendingMessage(review))
		// The modal stays open until the save succeeds, so a failure can show
		// the error without discarding the draft.
		m.reviewSubmitting = true
		save := m.beginLoading(submitReviewRatingCmd(m.ctx, m.app, m.reviewBook.Book.ID, rating, review, m.reviewSeq))
		return m, tea.Batch(save, toastCmd)
	}

	var cmd tea.Cmd
	if m.reviewFocus == dashboardReviewFocusRating {
		m.reviewRatingInput, cmd = m.reviewRatingInput.Update(msg)
	} else {
		m.reviewTextInput, cmd = m.reviewTextInput.Update(msg)
	}
	return m, cmd
}

// ── View ───────────────────────────────────────────────────────────────────

func (m dashboardModel) View() string {
	return m.fitToScreen(m.frame())
}

// frame renders the dashboard at its natural size. View clamps it to the
// terminal; keeping the two apart lets a test assert that the layout really
// fills the screen instead of being padded into it.
func (m dashboardModel) frame() string {
	if !m.loaded {
		return "\n  " + m.spin.View() + " Loading dashboard..."
	}

	statusBar := m.statusBar()

	// Body.
	var body string
	switch m.mode {
	case modeUpdatePage:
		body = statusBar + "\n" + m.renderLayout() + m.pagePrompt()
	default:
		body = statusBar + "\n" + m.renderLayout() + "\n" + m.contextHelpBar()
	}

	if m.mode == modeReviewRating {
		return m.overlayModal(m.reviewRatingOverlay())
	}
	if m.confirm.Active {
		return m.overlayModal(renderConfirmModal(m.confirm, max(36, min(60, m.width-10))))
	}
	if m.showHelp {
		return m.overlayModal(m.renderHelpModal())
	}
	return body
}

// statusBar renders the top bar: the app name and spinner on the left, the
// latest message on the right, over an unbroken background.
func (m dashboardModel) statusBar() string {
	width := max(20, m.width)
	inner := max(1, width-statusBarStyle.GetHorizontalPadding())

	left := statusBarTitleStyle.Render("oku")
	if m.isLoading() {
		left += statusBarAccentStyle.Render(" " + m.spin.View())
	}

	right := m.renderToast(inner - lipgloss.Width(left) - 2)

	gap := max(1, inner-lipgloss.Width(left)-lipgloss.Width(right))
	return statusBarStyle.Width(width).Render(
		left + statusBarFillStyle.Render(strings.Repeat(" ", gap)) + right,
	)
}

// renderToast draws the toast into avail columns: a glyph for the level (so
// a terminal without colour can tell an error from a note), the text cut to
// what fits, and the undo hint while there is a change to undo.
func (m dashboardModel) renderToast(avail int) string {
	if m.toast.text == "" {
		return ""
	}
	style, glyph := statusBarInfoStyle, ""
	switch m.toast.level {
	case toastWarn:
		style, glyph = statusBarWarnStyle, "! "
	case toastError:
		style, glyph = statusBarErrorStyle, "✗ "
	}
	undoHint := ""
	if m.undo != nil {
		undoHint = statusBarFillStyle.Render(" · ") +
			statusBarAccentStyle.Render("U") +
			statusBarFillStyle.Render(" undo")
	}

	// An API error can carry newlines and runs of whitespace. Left alone they
	// would wrap the bar onto rows the layout has not accounted for, and the
	// help bar would be the thing clipped off the bottom of the screen.
	text := strings.Join(strings.Fields(m.toast.text), " ")

	// A message wider than the bar would wrap it onto a second line and push
	// the layout down a row, so it is cut to the room that is left. The undo
	// hint goes first when there is not room for both.
	room := avail - lipgloss.Width(glyph) - lipgloss.Width(undoHint)
	if room < minStatusMessageWidth {
		undoHint = ""
		room = avail - lipgloss.Width(glyph)
	}
	if room < minStatusMessageWidth {
		return ""
	}
	return style.Render(glyph+ansi.Truncate(text, room, "…")) + undoHint
}

// fitToScreen pads the frame to the terminal and clamps it to those bounds, so
// an unusually long book title or error can never push the layout off-screen
// or wrap it onto a row that does not exist.
func (m dashboardModel) fitToScreen(frame string) string {
	if m.width <= 0 || m.height <= 0 {
		return frame
	}
	return lipgloss.NewStyle().
		MaxWidth(m.width).
		Height(m.height).
		MaxHeight(m.height).
		Render(frame)
}

// chromeRows counts the rows View draws outside the two-column layout: the
// status bar, plus whatever footer the current mode prints under it.
func (m dashboardModel) chromeRows() int {
	if m.mode == modeUpdatePage {
		return 1 + pagePromptRows
	}
	return 1 + 1 // status bar + help bar
}

// layoutHeight is the height of the two-column layout, borders included. It
// takes every row the chrome does not, so the panels reach the bottom of the
// terminal instead of leaving it blank.
func (m dashboardModel) layoutHeight() int {
	return max(8, m.height-m.chromeRows())
}

// rightPanelContentWidth mirrors renderLayout's width math for the right
// panel's content area.
func (m dashboardModel) rightPanelContentWidth() int {
	totalW := max(60, m.width-2)
	leftW := max(28, totalW*2/5)
	rightW := max(28, m.width-leftW-4)
	return rightW - 4
}

// rightPanelContentHeight mirrors renderLayout's height math for the right
// panel's content area.
func (m dashboardModel) rightPanelContentHeight() int {
	return max(1, m.layoutHeight()-2)
}

// renderLayout renders the 2-column layout: left sections + right context panel.
func (m dashboardModel) renderLayout() string {
	totalW := max(60, m.width-2)
	panelInnerH := m.rightPanelContentHeight()
	leftW := max(28, totalW*2/5)

	// The left frame is only a frame: the focus cue belongs to the card
	// inside it, or to the right pane when the cursor is over there.
	leftContent := clampPanelContent(m.renderSections(leftW-2, panelInnerH), leftW, panelInnerH)
	leftPanel := panelStyle.Width(leftW).Height(panelInnerH).Render(leftContent)

	// Right panel: context-sensitive.
	rightW := max(28, m.width-lipgloss.Width(leftPanel)-2)
	rightContent := clampPanelContent(m.rightPanelView(rightW-4), rightW, panelInnerH)
	rightStyle := panelStyle
	if m.rightPaneFocused() {
		rightStyle = panelFocusedStyle
	}
	rightPanel := rightStyle.
		Width(rightW).
		Height(panelInnerH).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

// rightPaneFocused reports whether j/k act on the right pane: over the search
// results, the timer's book picker or the stats page. The pane then carries
// the focus border, and the section card keeps its marker.
func (m dashboardModel) rightPaneFocused() bool {
	switch m.section {
	case sectionSearch:
		return m.searchSub == searchSubResults
	case sectionTimer:
		return m.timerSelecting && m.timerState == nil
	case sectionStats:
		return true
	}
	return false
}

// clampPanelContent pins a panel's content to its box: over-long lines are cut
// instead of wrapped, and content past the last row is dropped, so one long
// title can never stretch the layout past the bottom of the terminal.
func clampPanelContent(content string, w, h int) string {
	return lipgloss.NewStyle().
		MaxWidth(w).
		Height(h).
		MaxHeight(h).
		Render(content)
}

// renderSections renders the left panel content: section labels + expanded section.
func (m dashboardModel) renderSections(w, h int) string {
	defs := m.sectionDefinitions()
	if len(defs) == 0 {
		return ""
	}

	heights := m.leftSectionHeights(h)
	parts := make([]string, 0, len(defs))
	for _, def := range defs {
		// A zero-height card still costs a row once joined, so drop it.
		if heights[def.id] <= 0 {
			continue
		}
		parts = append(parts, m.renderSectionCard(def, w, heights[def.id], def.id == m.section))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m dashboardModel) sectionDefinitions() []sectionDef {
	return []sectionDef{
		{sectionIntro, "Intro", -1},
		{sectionReading, "Reading", len(m.readingBooks)},
		{sectionOku, "Oku", len(m.okuBooks)},
		{sectionSearch, "Search Titles", -1},
		{sectionStats, "Stats", -1},
		{sectionTimer, "Timer", -1},
	}
}

func (m dashboardModel) leftSectionHeights(totalH int) map[focusSection]int {
	heights := map[focusSection]int{
		sectionIntro:  3,
		sectionSearch: 4,
		sectionStats:  3,
		sectionTimer:  3,
	}
	minHeights := map[focusSection]int{
		sectionIntro:  2,
		sectionSearch: 3,
		sectionStats:  2,
		sectionTimer:  2,
	}
	// Intro gives up its box first: it is the one card whose whole content is
	// its label, so it is the one that loses nothing by being drawn bare.
	reduceOrder := []focusSection{
		sectionIntro, sectionStats, sectionTimer, sectionSearch,
	}

	fixedSum := heights[sectionIntro] + heights[sectionSearch] + heights[sectionStats] + heights[sectionTimer]
	remaining := totalH - fixedSum
	for remaining < 8 {
		changed := false
		for _, id := range reduceOrder {
			if remaining >= 8 {
				break
			}
			if heights[id] > minHeights[id] {
				heights[id]--
				fixedSum--
				remaining++
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	readingH := max(4, remaining*3/5)
	okuH := max(4, remaining-readingH)
	for readingH+okuH > remaining && (readingH > 2 || okuH > 2) {
		if readingH >= okuH && readingH > 2 {
			readingH--
		} else if okuH > 2 {
			okuH--
		}
	}
	for readingH+okuH < remaining {
		readingH++
	}

	if m.section == sectionReading && okuH > 4 {
		shift := min(2, okuH-4)
		okuH -= shift
		readingH += shift
	}
	if m.section == sectionOku && readingH > 4 {
		shift := min(2, readingH-4)
		readingH -= shift
		okuH += shift
	}

	heights[sectionReading] = readingH
	heights[sectionOku] = okuH

	sum := 0
	for _, def := range m.sectionDefinitions() {
		sum += heights[def.id]
	}
	if sum > totalH {
		deficit := sum - totalH
		shrinkOrder := []focusSection{
			sectionReading, sectionOku, sectionSearch, sectionIntro, sectionStats, sectionTimer,
		}
		for deficit > 0 {
			changed := false
			for _, id := range shrinkOrder {
				minH := 1
				if id == sectionReading || id == sectionOku {
					minH = 2
				}
				if heights[id] > minH {
					heights[id]--
					deficit--
					changed = true
					if deficit == 0 {
						break
					}
				}
			}
			if !changed {
				break
			}
		}
	}
	sum = 0
	for _, def := range m.sectionDefinitions() {
		sum += heights[def.id]
	}
	if sum < totalH {
		heights[sectionReading] += totalH - sum
	}

	return heights
}

func (m dashboardModel) renderSectionCard(def sectionDef, w, h int, focused bool) string {
	if h <= 0 {
		return ""
	}
	label := m.formatSectionLabel(def.id, def.label, def.count, focused)
	if h < 3 {
		// Too short to draw a border around: just the label.
		return clampPanelContent(label, w, h)
	}
	innerH := h - 2

	content := label
	if def.id == sectionReading || def.id == sectionOku || def.id == sectionSearch {
		// innerH > 1 leaves at least one row under the label.
		if innerH > 1 {
			if body := m.sectionContent(def.id, max(8, w-4)); body != "" {
				content += "\n" + body
			}
		}
	}

	style := panelStyle
	if focused {
		style = panelFocusedStyle
	}
	// A list whose items are taller than the rows it was given renders past
	// them; clip so the overflow cannot push the cards below this one off the
	// panel.
	clipped := clampPanelContent(content, w, innerH)
	clipped = stampOverflowBadge(clipped, m.listOverflowBadge(def.id), w)
	return style.Width(w).Height(innerH).Render(clipped)
}

// listOverflowBadge reports where the cursor sits in a list that shows fewer
// books than it holds. Hiding the pagination dots took away the only sign that
// there was anything below the last visible row.
func (m dashboardModel) listOverflowBadge(id focusSection) string {
	var l list.Model
	switch id {
	case sectionReading:
		l = m.readingList
	case sectionOku:
		l = m.okuList
	default:
		return ""
	}

	total := len(l.VisibleItems())
	if total == 0 || l.Paginator.PerPage >= total {
		return ""
	}
	return fmt.Sprintf("%d/%d", l.Index()+1, total)
}

// stampOverflowBadge right-aligns the badge on the card's last row, in the
// space the pagination dots used to take. The row is overwritten rather than
// appended to: a list pads its rows out to the full card width, so there is
// never anything left to append to.
func stampOverflowBadge(content, badge string, w int) string {
	if badge == "" {
		return content
	}
	badgeW := lipgloss.Width(badge)
	if w <= badgeW+2 {
		return content
	}

	lines := strings.Split(content, "\n")
	last := len(lines) - 1
	head := ansi.Truncate(lines[last], w-badgeW-1, "")
	if pad := w - badgeW - 1 - lipgloss.Width(head); pad > 0 {
		head += strings.Repeat(" ", pad)
	}
	lines[last] = head + dimStyleTUI.Render(badge)
	return strings.Join(lines, "\n")
}

func (m dashboardModel) formatSectionLabel(id focusSection, label string, count int, focused bool) string {
	num := fmt.Sprintf("%d", int(id)+1)
	countStr := ""
	if count >= 0 {
		countStr = sectionCountStyle.Render(fmt.Sprintf(" (%d)", count))
	}

	// Timer running indicator.
	if id == sectionTimer && m.timerState != nil {
		elapsed := time.Since(m.timerState.StartedAt)
		countStr = " " + keyStyle.Render(formatDuration(elapsed))
	}

	if focused {
		return sectionLabelFocusedStyle.Render("▸ "+num+"  "+label) + countStr
	}
	return sectionLabelStyle.Render("  "+num+"  "+label) + countStr
}

// sectionContent returns the expanded content for a focused section.
func (m dashboardModel) sectionContent(id focusSection, w int) string {
	switch id {
	case sectionReading:
		return m.readingList.View()
	case sectionOku:
		return m.okuList.View()
	case sectionSearch:
		return m.searchSectionContent(w)
	default:
		// Intro, Stats, Timer use the right pane for full details.
		return dimStyleTUI.Render("  See Output panel")
	}
}

func (m dashboardModel) searchSectionContent(w int) string {
	mode := dimStyleTUI.Render("[NORMAL]")
	if m.searchMode == searchModeInsert {
		mode = keyStyle.Render("[INSERT]")
	}
	queryMode := keyStyle.Render("[" + m.searchQueryMode.Label() + "]")
	return "  " + mode + " " + queryMode + " " + m.searchInput.View()
}

// ── Right Panel Views ──────────────────────────────────────────────────────

func (m dashboardModel) rightPanelView(w int) string {
	switch m.section {
	case sectionIntro:
		return m.introView(w)
	case sectionReading, sectionOku:
		return m.detailsView(w)
	case sectionSearch:
		return m.searchPanelView()
	case sectionStats:
		content, _ := clipLines(m.statsView(w), m.statsScroll, m.rightPanelContentHeight())
		return content
	case sectionTimer:
		return m.timerView(w)
	default:
		return ""
	}
}

func (m dashboardModel) detailsView(w int) string {
	b := m.selectedLibraryBook()
	if b == nil {
		return dimStyleTUI.Render("  No book selected")
	}

	var sb strings.Builder

	sb.WriteString(headStyle.Render(b.Book.Title))
	sb.WriteString("\n")
	author := fallback(b.Book.AuthorString(), "Unknown author")
	sb.WriteString(dimStyleTUI.Render(author))
	sb.WriteString("\n\n")

	writeField := func(label, value string) {
		sb.WriteString(labelStyle.Render(fmt.Sprintf("  %-10s ", label)))
		sb.WriteString(valueStyle.Render(value))
		sb.WriteString("\n")
	}

	writeField("Status", b.StatusID.Label())

	page := b.CurrentPage
	if len(b.UserBookReads) > 0 {
		page = b.UserBookReads[0].ProgressPages
	}
	progressText := b.Progress()
	if b.Book.Pages > 0 {
		// The field is 13 columns of label, then the text, two spaces, the bar
		// and " 100%". Size the bar to what is left so the row is never cut.
		barW := clampInt(w-20-lipgloss.Width(progressText), 8, 20)
		progressText += "  " + progressBar(page, b.Book.Pages, barW)
	}
	writeField("Progress", progressText)

	if m.density != densityCompact {
		writeField("Book ID", fmt.Sprintf("%d", b.Book.ID))
		if b.Book.Pages > 0 {
			writeField("Pages", fmt.Sprintf("%d", b.Book.Pages))
		}
		if b.Book.Rating > 0 {
			rating := fmt.Sprintf("%.2f", b.Book.Rating)
			if b.Book.RatingsCount > 0 {
				rating += fmt.Sprintf(" (%s ratings)", formatCount(b.Book.RatingsCount))
			}
			writeField("Rating", rating)
		}
		if b.Book.ReviewsCount > 0 {
			writeField("Reviews", formatCount(b.Book.ReviewsCount))
		}
		if b.Book.UsersReadCount > 0 || b.Book.UsersCount > 0 {
			readers := ""
			if b.Book.UsersReadCount > 0 {
				readers = formatCount(b.Book.UsersReadCount) + " read"
			}
			if b.Book.UsersCount > 0 {
				if readers != "" {
					readers += " · "
				}
				readers += formatCount(b.Book.UsersCount) + " shelved"
			}
			writeField("Readers", readers)
		}
		if b.Book.ReleaseDate != "" {
			writeField("Released", b.Book.ReleaseDate)
		}
		if b.Book.FeaturedSeries != "" {
			series := b.Book.FeaturedSeries
			if b.Book.FeaturedSeriesPosition > 0 {
				series += fmt.Sprintf(" #%d", b.Book.FeaturedSeriesPosition)
			}
			writeField("Series", series)
		}
	}

	if m.density == densityVerbose {
		if b.Book.Slug != "" {
			writeField("Slug", b.Book.Slug)
		}
		if len(b.UserBookReads) > 0 {
			if b.UserBookReads[0].StartedAt != nil {
				writeField("Started", b.UserBookReads[0].StartedAt.Format("2006-01-02"))
			}
			if b.UserBookReads[0].FinishedAt != nil {
				writeField("Finished", b.UserBookReads[0].FinishedAt.Format("2006-01-02"))
			}
		}
	}

	return sb.String()
}

func (m dashboardModel) searchPanelView() string {
	if m.searchLoading {
		query := m.searchLoadingQuery
		if strings.TrimSpace(query) == "" {
			query = m.lastQuery
		}
		if strings.TrimSpace(query) == "" {
			query = "..."
		}
		return "\n  " + m.spin.View() + " " + strings.ToLower(m.searchQueryMode.Label()) +
			" search (" + m.searchQueryMode.Description() + ") for " + fmt.Sprintf("%q", query)
	}

	if len(m.searchList.Items()) == 0 {
		if strings.TrimSpace(m.lastQuery) == "" {
			return dimStyleTUI.Render(
				fmt.Sprintf("  %s mode (%s). Type a query and press Enter.",
					strings.ToLower(m.searchQueryMode.Label()), m.searchQueryMode.Description(),
				),
			)
		}
		return dimStyleTUI.Render(fmt.Sprintf("  No results for %q", m.lastQuery))
	}

	return listHeaderStyle.Render(m.searchList.Title) + "\n" + m.searchList.View()
}

func (m *dashboardModel) openReviewRatingModal(book model.UserBook) {
	b := book
	m.reviewBook = &b
	m.mode = modeReviewRating
	m.showHelp = false
	m.reviewSubmitting = false
	m.reviewErr = ""
	m.reviewSeq++

	if b.Rating > 0 {
		m.reviewRatingInput.SetValue(fmt.Sprintf("%.1f", b.Rating))
	} else {
		m.reviewRatingInput.SetValue("")
	}
	m.reviewTextInput.SetValue(b.Review)
	m.focusReviewRatingField()
}

func (m *dashboardModel) closeReviewRatingModal() {
	m.mode = modeLibrary
	m.reviewBook = nil
	m.reviewSubmitting = false
	m.reviewErr = ""
	m.reviewSeq++
	m.reviewRatingInput.Blur()
	m.reviewTextInput.Blur()
}

// The focused field carries a marker in its label as well as the cursor, so
// the focus is visible on a terminal without colour.
const (
	reviewFieldFocused = "▸ "
	reviewFieldBlurred = "  "
)

func (m *dashboardModel) focusReviewRatingField() {
	m.reviewFocus = dashboardReviewFocusRating
	m.reviewRatingInput.Prompt = reviewFieldFocused + "Rating: "
	m.reviewRatingInput.Focus()
	m.reviewTextInput.Blur()
}

func (m *dashboardModel) focusReviewTextField() {
	m.reviewFocus = dashboardReviewFocusText
	m.reviewRatingInput.Prompt = reviewFieldBlurred + "Rating: "
	m.reviewRatingInput.Blur()
	m.reviewTextInput.Focus()
}

func (m dashboardModel) reviewRatingOverlay() string {
	if m.reviewBook == nil {
		return renderModalPanel("Review / Rate Book", modalDimStyle.Render("No book selected"), 48)
	}

	rating, err := parseReviewRatingInput(m.reviewRatingInput.Value())
	ratingLabel := "invalid rating"
	stars := "☆☆☆☆☆"
	if err == nil {
		stars = model.StarString(rating)
		ratingLabel = fmt.Sprintf("%.1f", rating)
	}

	var sb strings.Builder
	sb.WriteString(modalValueStyle.Render(m.reviewBook.Book.Title))
	sb.WriteString("\n")
	sb.WriteString(modalDimStyle.Render(fallback(m.reviewBook.Book.AuthorString(), "Unknown author")))
	sb.WriteString("\n\n")
	sb.WriteString(m.reviewRatingInput.View())
	sb.WriteString(modalBgStyle.Render("  "))
	sb.WriteString(modalKeyStyle.Render(stars))
	sb.WriteString(modalBgStyle.Render(" "))
	sb.WriteString(modalDimStyle.Render("(" + ratingLabel + ")"))
	sb.WriteString("\n\n")
	reviewMarker := reviewFieldBlurred
	if m.reviewFocus == dashboardReviewFocusText {
		reviewMarker = reviewFieldFocused
	}
	sb.WriteString(modalLabelStyle.Render(reviewMarker + "Review"))
	sb.WriteString("\n")
	sb.WriteString(m.reviewTextInput.View())
	sb.WriteString("\n\n")
	switch {
	case m.reviewSubmitting:
		sb.WriteString(modalDimStyle.Render("Saving..."))
		sb.WriteString("\n")
	case m.reviewErr != "":
		// The status bar sits behind the modal, so surface the failure here.
		sb.WriteString(modalErrorStyle.Render(m.reviewErr))
		sb.WriteString("\n")
	}

	sb.WriteString(modalDimStyle.Render("Tab/Shift+Tab switch fields   Ctrl+S save   Esc cancel"))

	width := max(70, m.width-10)
	if width > 100 {
		width = 100
	}
	return renderModalPanel("Review / Rate Book", sb.String(), width)
}

// ── Help ───────────────────────────────────────────────────────────────────

// openHelp shows the help modal, scrolled back to the top.
func (m *dashboardModel) openHelp() {
	m.showHelp = true
	m.syncHelpViewport()
	m.helpViewport.GotoTop()
}

// syncHelpViewport fits the help body to the terminal. The body is taller than
// a 40-row window, so it scrolls rather than spilling off the screen.
func (m *dashboardModel) syncHelpViewport() {
	body := m.helpModalBody()
	h := lipgloss.Height(body)
	if m.height > 0 {
		h = min(h, max(helpModalMinBodyRows, m.height-helpModalChromeRows-helpModalMarginRows))
	}

	offset := m.helpViewport.YOffset
	m.helpViewport = viewport.New(helpModalWidth-helpModalStyle.GetHorizontalPadding(), h)
	m.helpViewport.SetContent(body)
	m.helpViewport.SetYOffset(offset)
}

func (m dashboardModel) renderHelpModal() string {
	footer := "? or esc close"
	if m.helpViewport.TotalLineCount() > m.helpViewport.Height {
		footer = "j/k scroll · ? or esc close"
	}
	return renderModalPanel(
		"Help",
		m.helpModalRows()+"\n"+modalDimStyle.Render(footer),
		helpModalWidth,
	)
}

// helpModalRows returns the rows the help body currently shows. They are taken
// from the body rather than from viewport.View, which pads its own output with
// unstyled spaces: that padding would leave the panel striped, because only
// the modal style's own fill carries its background.
func (m dashboardModel) helpModalRows() string {
	lines := strings.Split(m.helpModalBody(), "\n")
	h := max(1, m.helpViewport.Height)
	start := clampInt(m.helpViewport.YOffset, 0, max(0, len(lines)-h))

	rows := make([]string, 0, h)
	rows = append(rows, lines[start:min(len(lines), start+h)]...)
	for len(rows) < h {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

// helpModalBody lists the keys the focus behind the modal understands, group
// by group, from the same bindings the handlers dispatch on.
func (m dashboardModel) helpModalBody() string {
	behind := m
	behind.showHelp = false
	k := behind.activeKeys()

	sections := make([]string, 0, 8)
	for _, g := range k.helpGroups() {
		rows := ""
		for _, b := range g.bindings {
			if !b.Enabled() {
				continue
			}
			// Every run carries the modal background, including the gaps: a
			// style that only sets a foreground ends with a reset, which would
			// stripe the row with the terminal's own background.
			rows += modalBgStyle.Render("  ") +
				modalKeyStyle.Width(12).Render(b.Help().Key) +
				modalBgStyle.Render("  ") +
				modalDescStyle.Render(b.Help().Desc) + "\n"
		}
		if rows == "" {
			continue
		}
		sections = append(sections, modalHeadStyle.Render(g.title)+"\n"+rows)
	}

	// Joined by hand: lipgloss.JoinVertical pads every row out to the widest
	// one with unstyled spaces, and those spaces would show as bands of
	// terminal background across the modal.
	return strings.TrimRight(strings.Join(sections, "\n"), "\n")
}

func (m dashboardModel) overlayModal(modal string) string {
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		modal,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("0")),
	)
}

// ── Keymap ─────────────────────────────────────────────────────────────────

// keyMap binds every action to its keys, once. The handlers dispatch through
// key.Matches on these bindings, and the help bar and the help modal are
// generated from them, so the two cannot drift. newKeyMap returns them all
// disabled; activeKeys enables the ones the current focus understands and
// relabels the few whose meaning depends on it.
type keyMap struct {
	// Everywhere.
	Quit      key.Binding
	ForceQuit key.Binding // ctrl+c: works in every mode, never advertised
	Help      key.Binding
	Back      key.Binding // esc: back, cancel, close - whatever fits the focus
	Undo      key.Binding

	// Moving around.
	Up, Down                 key.Binding
	PrevSection, NextSection key.Binding
	Search                   key.Binding
	ScrollTop, ScrollBottom  key.Binding
	HalfPageUp, HalfPageDown key.Binding

	// Library.
	Details                                              key.Binding
	ProgressUp, ProgressDown                             key.Binding
	Update, Rate                                         key.Binding
	SetReading, SetWant, SetFinished, SetDNF, SetIgnored key.Binding
	Timer, TimerStop                                     key.Binding
	Sync, Refresh, Density                               key.Binding

	// Search.
	SearchInsert, SearchAppend                        key.Binding
	SearchMode                                        key.Binding
	SearchModeBook, SearchModeAuthor, SearchModeGenre key.Binding
	SearchSubmit, AddReading                          key.Binding
	SearchBack                                        key.Binding // results back to the input

	// Pickers and modals.
	Select                           key.Binding // enter: the timer picker, a confirm button, the page prompt
	ConfirmYes, ConfirmNo            key.Binding
	ConfirmLeft, ConfirmRight        key.Binding
	ReviewSave                       key.Binding
	ReviewNextField, ReviewPrevField key.Binding

	// short is the help bar for the current focus, in the order the hints
	// are reached for. Built by activeKeys.
	short []key.Binding
}

// helpGroup is one titled block of the help modal.
type helpGroup struct {
	title    string
	bindings []key.Binding
}

// bind makes a disabled binding: activeKeys turns on the ones that apply.
func bind(label, desc string, keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(label, desc), key.WithDisabled())
}

func newKeyMap() keyMap {
	return keyMap{
		Quit:      bind("q", "quit", "q"),
		ForceQuit: bind("ctrl+c", "quit", "ctrl+c"),
		Help:      bind("?", "help", "?"),
		Back:      bind("Esc", "back", "esc"),
		Undo:      bind("U", "undo the last change", "U"),

		Up:           bind("k", "up", "k", "up"),
		Down:         bind("j", "down", "j", "down"),
		PrevSection:  bind("h", "previous section", "h", "left", "shift+tab"),
		NextSection:  bind("l", "next section", "l", "right", "tab"),
		Search:       bind("/", "search", "/"),
		ScrollTop:    bind("g", "top", "g", "home"),
		ScrollBottom: bind("G", "bottom", "G", "end"),
		HalfPageUp:   bind("ctrl+u", "half page up", "ctrl+u", "pgup"),
		HalfPageDown: bind("ctrl+d", "half page down", "ctrl+d", "pgdown"),

		Details:      bind("↵", "details", "enter"),
		ProgressUp:   bind("+", "+10 pages", "+", "="),
		ProgressDown: bind("-", "-10 pages", "-"),
		Update:       bind("u", "update page", "u"),
		Rate:         bind("v", "review / rate", "v"),
		SetReading:   bind("g", "set reading", "g"),
		SetWant:      bind("w", "set want to read", "w"),
		SetFinished:  bind("f", "set finished", "f"),
		SetDNF:       bind("d", "did not finish (asks)", "d"),
		SetIgnored:   bind("x", "ignore / remove (asks)", "x"),
		Timer:        bind("t", "start timer", "t"),
		TimerStop:    bind("s", "stop timer", "s"),
		Sync:         bind("s", "sync with Hardcover", "s"),
		Refresh:      bind("r", "refresh", "r"),
		Density:      bind("z", "density", "z"),

		SearchInsert:     bind("i", "insert", "i"),
		SearchAppend:     bind("a", "append", "a"),
		SearchMode:       bind("m", "cycle mode", "m"),
		SearchModeBook:   bind("1", "book mode", "1"),
		SearchModeAuthor: bind("2", "author mode", "2"),
		SearchModeGenre:  bind("3", "genre mode", "3"),
		SearchSubmit:     bind("↵", "search", "enter"),
		AddReading:       bind("↵", "add as reading", "enter"),
		SearchBack:       bind("Esc", "back to input", "esc", "h", "left"),

		Select: bind("↵", "select", "enter"),
		// The dialog has always answered to shifted letters too.
		ConfirmYes:      bind("y", "yes, do it", "y", "Y"),
		ConfirmNo:       bind("n", "no, leave it", "n", "N", "esc"),
		ConfirmLeft:     bind("h", "pick left", "h", "left", "k", "up", "H", "K"),
		ConfirmRight:    bind("l", "pick right", "l", "right", "j", "down", "L", "J"),
		ReviewSave:      bind("ctrl+s", "save", "ctrl+s"),
		ReviewNextField: bind("Tab", "next field", "tab"),
		ReviewPrevField: bind("Shift+Tab", "previous field", "shift+tab"),
	}
}

// enable turns bindings on.
func enable(bs ...*key.Binding) {
	for _, b := range bs {
		b.SetEnabled(true)
	}
}

// hint folds bindings into one help entry ("j/k navigate"). It is display
// only: its keys are exactly its parts' keys, its label their labels joined,
// and it is enabled only while every part is.
func hint(desc string, parts ...key.Binding) key.Binding {
	labels := make([]string, 0, len(parts))
	for _, p := range parts {
		labels = append(labels, p.Help().Key)
	}
	return hintAs(strings.Join(labels, "/"), desc, parts...)
}

// hintAs is hint with an explicit label, for parts whose labels do not join
// into a readable one ("Tab/Shift+Tab", "n/Esc").
func hintAs(label, desc string, parts ...key.Binding) key.Binding {
	keys := make([]string, 0, 2*len(parts))
	enabled := true
	for _, p := range parts {
		keys = append(keys, p.Keys()...)
		enabled = enabled && p.Enabled()
	}
	b := key.NewBinding(key.WithKeys(keys...), key.WithHelp(label, desc))
	b.SetEnabled(enabled)
	return b
}

// ShortHelp is the help bar: the hints for the current focus, in order.
func (k keyMap) ShortHelp() []key.Binding {
	out := make([]key.Binding, 0, len(k.short))
	for _, b := range k.short {
		if b.Enabled() {
			out = append(out, b)
		}
	}
	return out
}

// FullHelp is every enabled binding, grouped as the help modal shows them.
func (k keyMap) FullHelp() [][]key.Binding {
	groups := k.helpGroups()
	out := make([][]key.Binding, 0, len(groups))
	for _, g := range groups {
		col := make([]key.Binding, 0, len(g.bindings))
		for _, b := range g.bindings {
			if b.Enabled() {
				col = append(col, b)
			}
		}
		if len(col) > 0 {
			out = append(out, col)
		}
	}
	return out
}

// helpGroups is the help modal's structure. Every binding is listed; the
// ones the current focus has not enabled are skipped when it is drawn.
func (k keyMap) helpGroups() []helpGroup {
	return []helpGroup{
		{"Navigation", []key.Binding{
			hint(k.Down.Help().Desc, k.Down, k.Up),
			hint("section", k.PrevSection, k.NextSection),
			hintAs("Tab/S-Tab", "section (alias)", k.NextSection, k.PrevSection),
			k.Search,
			k.Back,
			k.SearchBack,
			k.ScrollTop,
			k.ScrollBottom,
			hint("half page", k.HalfPageUp, k.HalfPageDown),
		}},
		{"Actions", []key.Binding{
			k.Details,
			k.AddReading,
			k.SearchSubmit,
			k.Select,
			hint("page ±10", k.ProgressUp, k.ProgressDown),
			k.Update,
			k.Rate,
			k.SetReading,
			k.SetWant,
			k.SetFinished,
			k.SetDNF,
			k.SetIgnored,
			hint("insert", k.SearchInsert, k.SearchAppend),
			k.SearchMode,
			hint("book / author / genre", k.SearchModeBook, k.SearchModeAuthor, k.SearchModeGenre),
			k.Density,
		}},
		{"Timer", []key.Binding{k.Timer, k.TimerStop}},
		{"Data", []key.Binding{k.Refresh, k.Sync}},
		{"Confirm", []key.Binding{
			k.ConfirmYes,
			hintAs("n/Esc", k.ConfirmNo.Help().Desc, k.ConfirmNo),
			hint("pick a button", k.ConfirmLeft, k.ConfirmRight),
		}},
		{"Review", []key.Binding{
			k.ReviewSave,
			hint("switch field", k.ReviewNextField, k.ReviewPrevField),
		}},
		{"General", []key.Binding{k.Help, k.Undo, k.Quit}},
	}
}

// activeKeys is the keymap for what has focus right now: the bindings that
// apply are enabled, the ones whose label depends on the focus are relabelled,
// and the help bar's order is set. Everything else stays disabled, so a
// disabled key neither matches in a handler nor shows in the help.
func (m dashboardModel) activeKeys() keyMap {
	k := newKeyMap()
	enable(&k.ForceQuit)

	switch {
	case m.confirm.Active:
		k.Select.SetHelp("↵", "choose")
		enable(&k.ConfirmYes, &k.ConfirmNo, &k.ConfirmLeft, &k.ConfirmRight, &k.Select)
		k.short = []key.Binding{k.ConfirmYes, hintAs("n/Esc", "no", k.ConfirmNo), hint("pick", k.ConfirmLeft, k.ConfirmRight)}
		return k

	case m.mode == modeUpdatePage:
		k.Back.SetHelp("Esc", "cancel")
		k.Select.SetHelp("↵", "save")
		enable(&k.Back, &k.Select)
		k.short = []key.Binding{k.Select, k.Back}
		return k

	case m.mode == modeReviewRating:
		k.Back.SetHelp("Esc", "cancel")
		enable(&k.Back)
		if !m.reviewSubmitting {
			enable(&k.ReviewSave, &k.ReviewNextField, &k.ReviewPrevField)
		}
		k.short = []key.Binding{hint("switch field", k.ReviewNextField, k.ReviewPrevField), k.ReviewSave, k.Back}
		return k

	case m.showHelp:
		k.Up.SetHelp("k", "scroll")
		k.Down.SetHelp("j", "scroll")
		k.Back.SetHelp("Esc", "close")
		enable(&k.Help, &k.Back, &k.Quit, &k.Up, &k.Down,
			&k.HalfPageUp, &k.HalfPageDown, &k.ScrollTop, &k.ScrollBottom)
		k.short = []key.Binding{hint("scroll", k.Down, k.Up), k.Back}
		return k
	}

	sectionHint := hint("section", k.PrevSection, k.NextSection)

	// Undo lasts as long as the toast that offers it, and never takes a
	// letter away from the search input.
	typing := m.section == sectionSearch && m.searchSub == searchSubInput && m.searchMode == searchModeInsert
	if m.undo != nil && !typing {
		enable(&k.Undo)
	}

	switch m.section {
	case sectionReading, sectionOku:
		k.Up.SetHelp("k", "navigate")
		k.Down.SetHelp("j", "navigate")
		if m.timerState != nil {
			k.Timer.SetHelp("t", "stop timer")
		} else if m.section != sectionReading {
			k.Timer.SetHelp("t", "timer (Reading list)")
		}
		enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.NextSection, &k.PrevSection, &k.Search,
			&k.Details, &k.ProgressUp, &k.ProgressDown, &k.Update, &k.Rate,
			&k.SetReading, &k.SetWant, &k.SetFinished, &k.SetDNF, &k.SetIgnored,
			&k.Timer, &k.Sync, &k.Refresh, &k.Density)

		// Ordered by how often a key is reached for, with help first so it is
		// the one hint a narrow terminal never drops. Enter is left out: the
		// detail pane it opens is already on screen.
		k.short = []key.Binding{
			k.Help,
			hint("navigate", k.Down, k.Up),
			sectionHint,
			hint("status", k.SetReading, k.SetWant, k.SetFinished, k.SetDNF, k.SetIgnored),
			hint("page", k.ProgressUp, k.ProgressDown),
			hintAs("u", "update", k.Update),
		}
		if m.section == sectionReading || m.timerState != nil {
			k.short = append(k.short, k.Timer)
		}
		// The bar has a word per key; the modal spells them out.
		k.short = append(k.short, k.Search, hintAs("v", "rate", k.Rate), hintAs("s", "sync", k.Sync), k.Density, k.Refresh)

	case sectionSearch:
		switch {
		case m.searchSub == searchSubResults:
			k.Up.SetHelp("k", "navigate")
			k.Down.SetHelp("j", "navigate")
			k.SetReading.SetHelp("g", "add as reading")
			k.SetWant.SetHelp("w", "add as want to read")
			k.SetFinished.SetHelp("f", "add as finished")
			k.SetDNF.SetHelp("d", "add as did not finish")
			enable(&k.Help, &k.Up, &k.Down, &k.AddReading, &k.SetReading, &k.SetWant, &k.SetFinished,
				&k.SetDNF, &k.SearchBack, &k.NextSection, &k.PrevSection, &k.Density)
			k.short = []key.Binding{
				k.Help,
				hint("navigate", k.Down, k.Up),
				k.AddReading,
				hint("status", k.SetReading, k.SetWant, k.SetFinished, k.SetDNF),
				hintAs("h/l", "input/next", k.SearchBack, k.NextSection),
				k.Density,
				k.SearchBack,
			}
		case m.searchMode == searchModeInsert:
			// ? is typed here, not a shortcut.
			k.Back.SetHelp("Esc", "normal")
			enable(&k.SearchSubmit, &k.Back)
			k.short = []key.Binding{k.SearchSubmit, k.Back}
		default:
			k.Up.SetHelp("k", "previous section")
			k.Down.SetHelp("j", "results / next section")
			enable(&k.Help, &k.SearchInsert, &k.SearchAppend, &k.SearchMode,
				&k.SearchModeBook, &k.SearchModeAuthor, &k.SearchModeGenre,
				&k.Density, &k.SearchSubmit, &k.NextSection, &k.PrevSection, &k.Back, &k.Up, &k.Down)
			k.short = []key.Binding{
				k.Help,
				k.SearchSubmit,
				hint("insert", k.SearchInsert, k.SearchAppend),
				hintAs("m", "mode", k.SearchMode),
				hint("book/author/genre", k.SearchModeBook, k.SearchModeAuthor, k.SearchModeGenre),
				sectionHint,
				k.Density,
				k.Back,
			}
		}

	case sectionStats:
		k.Up.SetHelp("k", "scroll")
		k.Down.SetHelp("j", "scroll")
		enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.ScrollTop, &k.NextSection, &k.PrevSection,
			&k.Sync, &k.Refresh, &k.Search)
		k.short = []key.Binding{
			k.Help, hint("scroll", k.Down, k.Up), k.ScrollTop, sectionHint,
			hintAs("s", "sync", k.Sync), k.Refresh, k.Search, k.Quit,
		}

	case sectionTimer:
		switch {
		case m.timerSelecting && m.timerState == nil:
			k.Up.SetHelp("k", "choose")
			k.Down.SetHelp("j", "choose")
			k.Select.SetHelp("↵", "start")
			k.Back.SetHelp("Esc", "cancel")
			enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.Select, &k.Back)
			k.short = []key.Binding{k.Help, hint("choose", k.Down, k.Up), k.Select, k.Back, k.Quit}
		case m.timerState != nil:
			k.Up.SetHelp("k", "previous section")
			k.Down.SetHelp("j", "next section")
			k.Timer.SetHelp("t", "stop timer")
			enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.NextSection, &k.PrevSection, &k.Search,
				&k.Timer, &k.TimerStop)
			k.short = []key.Binding{k.Help, hint("stop", k.Timer, k.TimerStop), sectionHint, k.Search, k.Quit}
		default:
			k.Up.SetHelp("k", "previous section")
			k.Down.SetHelp("j", "next section")
			k.Timer.SetHelp("t", "choose + start")
			enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.NextSection, &k.PrevSection, &k.Search, &k.Timer)
			k.short = []key.Binding{k.Help, k.Timer, sectionHint, k.Search, k.Quit}
		}

	default:
		k.Up.SetHelp("k", "previous section")
		k.Down.SetHelp("j", "next section")
		enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.NextSection, &k.PrevSection, &k.Search)
		k.short = []key.Binding{k.Help, sectionHint, hintAs("Tab", "next", k.NextSection), k.Search, k.Quit}
	}
	return k
}

// helpBarWidth is the room the footer hints have. Zero means unbounded, which
// is what a model that has not seen a window size yet should use.
func (m dashboardModel) helpBarWidth() int {
	if m.width <= 0 {
		return 0
	}
	return max(minHelpBarWidth, m.width-2)
}

// contextHelpBar renders the hints for whatever has focus, on one line.
func (m dashboardModel) contextHelpBar() string {
	return " " + m.renderHelpBar(m.helpBindings())
}

// renderHelpBar lays out as many hints as fit and marks the rest with an
// ellipsis. help.Model can do this itself, but when the ellipsis is the thing
// that does not fit its width check falls through and appends the hint anyway,
// leaving a dangling separator to be cut mid-word; the bar drops whole hints
// here instead, so the width is always honoured and the cut is always marked.
func (m dashboardModel) renderHelpBar(bindings []key.Binding) string {
	h := m.help
	h.Width = 0 // The loop below owns the width.

	view := h.ShortHelpView(bindings)
	limit := m.helpBarWidth()
	if limit <= 0 || lipgloss.Width(view)+2 <= limit {
		return view
	}

	ellipsis := dimStyleTUI.Render("…")
	for n := len(bindings) - 1; n >= 1; n-- {
		candidate := h.ShortHelpView(bindings[:n]) + " " + ellipsis
		if lipgloss.Width(candidate)+2 <= limit {
			return candidate
		}
	}
	return ellipsis
}

// helpBindings returns the hints for the focused section.
func (m dashboardModel) helpBindings() []key.Binding {
	return m.activeKeys().ShortHelp()
}

// ── Navigation helpers ─────────────────────────────────────────────────────

// setSection focuses a section and re-sizes the lists. leftSectionHeights
// gives the focused list extra rows, so the sizes have to follow the focus and
// not only a window resize.
func (m *dashboardModel) setSection(s focusSection) {
	m.section = s
	m.resize()
}

func (m *dashboardModel) nextSection() {
	m.searchInput.Blur()
	m.setSection((m.section + 1) % sectionCount)
}

func (m *dashboardModel) prevSection() {
	m.searchInput.Blur()
	m.setSection((m.section - 1 + sectionCount) % sectionCount)
}

// ── Search helpers ─────────────────────────────────────────────────────────

func (m dashboardModel) hasSearchResults() bool {
	return len(m.searchList.Items()) > 0
}

// submitSearch starts a search. An in-flight search is not a reason to
// refuse: searchSeq drops whichever result is superseded, so a typo can be
// corrected immediately.
func (m *dashboardModel) submitSearch() tea.Cmd {
	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		return m.showToast(toastError, "search query cannot be empty")
	}

	// Reuse in-memory results for the same query instead of refetching.
	if strings.EqualFold(query, strings.TrimSpace(m.lastQuery)) &&
		m.searchQueryMode == m.lastSearchMode && len(m.searchList.Items()) > 0 {
		m.searchSub = searchSubResults
		m.searchMode = searchModeNormal
		m.searchInput.Blur()
		return m.showToast(toastInfo, fmt.Sprintf("%s mode: showing cached results for %q",
			strings.ToLower(m.searchQueryMode.Label()),
			query,
		))
	}

	m.searchLoading = true
	m.searchLoadingQuery = query

	toastCmd := m.showToast(toastInfo, fmt.Sprintf("%s mode (%s): searching for %q...",
		strings.ToLower(m.searchQueryMode.Label()),
		m.searchQueryMode.Description(),
		query,
	))
	m.refreshSearchTitle()
	m.searchSeq++
	search := m.beginLoading(searchCmd(m.ctx, m.app, query, m.searchQueryMode, m.searchSeq))
	return tea.Batch(search, toastCmd)
}

func (m *dashboardModel) cycleDensity() tea.Cmd {
	switch m.density {
	case densityCompact:
		m.density = densityDefault
	case densityDefault:
		m.density = densityVerbose
	default:
		m.density = densityCompact
	}
	cmd := tea.Batch(m.refreshListItems(), m.refreshSearchResultItems())
	return tea.Batch(cmd, m.showToast(toastInfo, "Density: "+densityLabel(m.density)))
}

func densityLabel(d outputDensity) string {
	switch d {
	case densityCompact:
		return "compact"
	case densityVerbose:
		return "verbose"
	default:
		return "default"
	}
}

// setSearchQueryMode switches the mode the next search runs in and returns
// the toast that says so.
func (m *dashboardModel) setSearchQueryMode(mode model.SearchMode) tea.Cmd {
	if mode == "" {
		mode = model.SearchModeBook
	}
	// Picking the mode that is already set still says so: the key did
	// something, and the toast is the only place the mode is spelled out.
	m.searchQueryMode = mode
	// The results on screen were fetched in the old mode, so the header is
	// left naming them; only the next search renames it.
	m.refreshSearchTitle()
	m.searchInput.Placeholder = searchPlaceholderForMode(mode)
	m.updateSearchSuggestions()
	return m.showToast(toastInfo, fmt.Sprintf("Search mode: %s (%s)", strings.ToLower(mode.Label()), mode.Description()))
}

// addRecentSearchQuery puts a query at the head of the history and returns the
// command that writes it back to the store.
func (m *dashboardModel) addRecentSearchQuery(query string) tea.Cmd {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	m.recentSearches = dedupeQueries(append([]string{query}, m.recentSearches...))
	return saveRecentSearchesCmd(m.app, m.recentSearches)
}

// mergeRecentSearches keeps this session's queries ahead of the ones read back
// from the store, so a load landing mid-session cannot drop them.
func (m *dashboardModel) mergeRecentSearches(loaded []string) {
	m.recentSearches = dedupeQueries(append(append([]string(nil), m.recentSearches...), loaded...))
}

// dedupeQueries keeps the first spelling of each query, compared without case,
// and caps the history.
func dedupeQueries(queries []string) []string {
	seen := make(map[string]struct{}, len(queries))
	out := make([]string, 0, min(len(queries), maxRecentSearches))
	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		key := strings.ToLower(q)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, q)
		if len(out) == maxRecentSearches {
			break
		}
	}
	return out
}

func encodeRecentSearches(queries []string) (string, error) {
	raw, err := json.Marshal(dedupeQueries(queries))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeRecentSearches(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var queries []string
	if err := json.Unmarshal([]byte(raw), &queries); err != nil {
		// A corrupt value is not worth reporting: the history is a nicety.
		return nil
	}
	return dedupeQueries(queries)
}

func (m *dashboardModel) updateSearchSuggestions() {
	seen := map[string]struct{}{}
	out := make([]string, 0, 12)

	push := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}

	for _, q := range m.recentSearches {
		push(q)
	}
	if m.searchQueryMode == model.SearchModeAuthor {
		for _, b := range append(append([]model.UserBook(nil), m.readingBooks...), m.okuBooks...) {
			for _, a := range b.Book.Authors {
				push(a)
			}
		}
	}

	if len(out) > 12 {
		out = out[:12]
	}
	m.searchInput.SetSuggestions(out)
}

func searchPlaceholderForMode(mode model.SearchMode) string {
	switch mode {
	case model.SearchModeAuthor:
		return "Search by author name..."
	case model.SearchModeGenre:
		return "Search by genre or tag..."
	default:
		return "Search books..."
	}
}

// refreshSearchTitle names the results the panel is actually showing: the mode
// they were fetched with and how many came back, not the mode the user has
// switched to since.
func (m *dashboardModel) refreshSearchTitle() {
	if m.searchLoading {
		m.searchList.Title = fmt.Sprintf("%s Results (loading...)", m.searchQueryMode.Label())
		return
	}
	mode := m.lastSearchMode
	if mode == "" {
		mode = m.searchQueryMode
	}
	m.searchList.Title = fmt.Sprintf("%s Results (%d)", mode.Label(), len(m.searchBooks))
}

func (m *dashboardModel) enterSearchNormalMode() {
	m.searchMode = searchModeNormal
	m.searchInput.Blur()
}

func (m *dashboardModel) enterSearchInsertMode() {
	m.searchMode = searchModeInsert
	m.searchInput.Focus()
}

// ── Resize ─────────────────────────────────────────────────────────────────

func (m *dashboardModel) resize() {
	m.help.Width = m.helpBarWidth()

	totalW := max(60, m.width-2)
	panelInnerH := m.rightPanelContentHeight()

	leftW := max(28, totalW*2/5)
	rightW := max(28, totalW-leftW-3)

	heights := m.leftSectionHeights(panelInnerH)
	readingContentH := max(1, heights[sectionReading]-3)
	okuContentH := max(1, heights[sectionOku]-3)
	leftContentW := leftW - 6
	if leftContentW < 8 {
		leftContentW = 8
	}

	// "[NORMAL] [BOOK] / " eats the front of the search card's row; the input
	// takes what is left instead of being cut off mid-placeholder.
	m.searchInput.Width = max(4, leftContentW-20)

	m.readingList.SetSize(leftContentW, readingContentH)
	m.okuList.SetSize(leftContentW, okuContentH)
	m.searchList.SetSize(rightW-4, max(1, panelInnerH-1))

	if m.showHelp {
		m.syncHelpViewport()
	}
}

// ── List helpers ───────────────────────────────────────────────────────────

// refreshListItems rebuilds both library lists. The returned command must be
// run: with filtering enabled, SetItems reapplies an active filter.
func (m *dashboardModel) refreshListItems() tea.Cmd {
	toItems := func(books []model.UserBook) []list.Item {
		items := make([]list.Item, 0, len(books))
		for _, b := range books {
			items = append(items, userBookItem{
				book:    b,
				density: m.density,
			})
		}
		return items
	}
	readingCmd := m.readingList.SetItems(toItems(m.readingBooks))
	okuCmd := m.okuList.SetItems(toItems(m.okuBooks))
	return tea.Batch(readingCmd, okuCmd)
}

func (m *dashboardModel) refreshSearchResultItems() tea.Cmd {
	m.applySearchListDensityLayout()

	idx := m.searchList.Index()
	items := make([]list.Item, 0, len(m.searchBooks))
	for _, r := range m.searchBooks {
		items = append(items, searchResultItem{
			result:  r,
			density: m.density,
		})
	}
	cmd := m.searchList.SetItems(items)
	if len(items) == 0 {
		return cmd
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(items) {
		idx = len(items) - 1
	}
	m.searchList.Select(idx)
	return cmd
}

func (m *dashboardModel) applySearchListDensityLayout() {
	spacing := 0
	if m.density == densityVerbose {
		spacing = 1
	}
	m.searchList.SetDelegate(newListDelegate(spacing))
}

// newListDelegate is the item renderer every list shares: title over
// description, the selection marked by a bar in the accent colour.
func newListDelegate(spacing int) list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(spacing)

	delegate.Styles.NormalTitle = lipgloss.NewStyle().
		Foreground(th.text).
		Padding(0, 0, 0, 2)
	delegate.Styles.NormalDesc = lipgloss.NewStyle().
		Foreground(th.textMuted).
		Padding(0, 0, 0, 2)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(th.accent).
		Foreground(th.accent).
		Bold(true).
		Padding(0, 0, 0, 1)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(th.accent).
		Foreground(th.textMuted).
		Padding(0, 0, 0, 1)
	delegate.Styles.DimmedTitle = lipgloss.NewStyle().
		Foreground(th.textDim).
		Padding(0, 0, 0, 2)
	delegate.Styles.DimmedDesc = lipgloss.NewStyle().
		Foreground(th.border).
		Padding(0, 0, 0, 2)
	return delegate
}

func (m dashboardModel) selectedLibraryBook() *model.UserBook {
	var item list.Item
	if m.section == sectionOku {
		item = m.okuList.SelectedItem()
	} else {
		item = m.readingList.SelectedItem()
	}
	if item == nil {
		return nil
	}
	ub, ok := item.(userBookItem)
	if !ok {
		return nil
	}
	book := ub.book
	return &book
}

func (m dashboardModel) selectedSearchResult() *model.SearchResult {
	item := m.searchList.SelectedItem()
	if item == nil {
		return nil
	}
	sr, ok := item.(searchResultItem)
	if !ok {
		return nil
	}
	r := sr.result
	return &r
}

func (m dashboardModel) changeSelectedLibraryStatus(status model.Status) (tea.Model, tea.Cmd) {
	b := m.selectedLibraryBook()
	if b == nil {
		return m, m.showToast(toastError, "no book selected")
	}
	return m.startOp(changeStatusCmd(m.ctx, m.app, b.Book.ID, b.Book.Title, b.StatusID, status))
}

func (m dashboardModel) changeSelectedSearchStatus(status model.Status) (tea.Model, tea.Cmd) {
	r := m.selectedSearchResult()
	if r == nil {
		return m, m.showToast(toastError, "no search result selected")
	}
	return m.startOp(addFromSearchCmd(m.ctx, m.app, r.ID, status))
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

func loadLocalDataCmd(a *app.App) tea.Cmd {
	return func() tea.Msg {
		if a == nil {
			return localDataLoadedMsg{err: fmt.Errorf("app not initialized")}
		}
		stats, err := a.GetReadingStats()
		if err != nil {
			return localDataLoadedMsg{err: err}
		}
		sessions, err := a.TimerList(5)
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
			stats, sessions = demoLocalData()
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
		}

	}
}

// shouldUseDemoLocalData reports whether to show fabricated dashboard data.
// Opt-in only (for demo recordings): an empty database is a real state and a
// DB failure must surface as an error, never as fake numbers.
func shouldUseDemoLocalData() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("OKU_TUI_DEMO_DATA")), "1")
}

func demoLocalData() (*model.ReadingStats, []model.ReadingSession) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	heatmap := make([]model.DayActivity, 0, 26*7)
	for i := 0; i < 26*7; i++ {
		d := today.AddDate(0, 0, -i)
		weekday := (int(d.Weekday()) + 6) % 7 // Mon=0..Sun=6
		base := []int{28, 35, 22, 44, 30, 55, 18}[weekday]
		variation := (i * 7) % 26
		mins := base + variation
		if i%9 == 0 || i%17 == 0 {
			mins = 0
		}
		heatmap = append(heatmap, model.DayActivity{
			Date:    d,
			Minutes: mins,
		})
	}

	weeklyDays := [7]int{36, 52, 28, 64, 41, 73, 34}
	total := 0
	longestIdx := 0
	for i, m := range weeklyDays {
		total += m
		if m > weeklyDays[longestIdx] {
			longestIdx = i
		}
	}
	stats := model.WeeklyStats{
		Days:       weeklyDays,
		Total:      total,
		Sessions:   11,
		LongestDay: longestIdx,
	}

	makeSession := func(daysAgo, startHour, minutes, bookID int, title string) model.ReadingSession {
		start := today.AddDate(0, 0, -daysAgo).Add(time.Duration(startHour) * time.Hour).Add(12 * time.Minute)
		end := start.Add(time.Duration(minutes) * time.Minute)
		return model.ReadingSession{
			ID:        daysAgo + 1,
			BookID:    bookID,
			StartedAt: start,
			EndedAt:   &end,
			BookTitle: title,
		}
	}
	sessions := []model.ReadingSession{
		makeSession(0, 20, 48, 101, "The Essential Kafka"),
		makeSession(1, 19, 36, 102, "The Communist Manifesto"),
		makeSession(2, 21, 62, 103, "Meditations"),
		makeSession(3, 18, 29, 104, "Dune"),
		makeSession(4, 20, 55, 105, "Atomic Habits"),
	}

	goalEnd := time.Date(now.Year(), 12, 31, 0, 0, 0, 0, now.Location())
	readingStats := &model.ReadingStats{
		Year: model.YearSummary{
			Year:          now.Year(),
			BooksFinished: 12,
			PagesRead:     3748,
			AvgRating:     3.9,
			RatedCount:    9,
		},
		Goal: &model.Goal{
			ID:       1,
			Metric:   "books",
			Target:   20,
			Progress: 12,
			State:    "active",
			EndDate:  goalEnd,
		},
		Months: [12]int{3, 2, 0, 4, 1, 2, 0, 0, 0, 0, 0, 0},
		Years: []model.LabelCount{
			{Label: "2023", Count: 14}, {Label: "2024", Count: 21},
			{Label: "2025", Count: 18}, {Label: fmt.Sprintf("%d", now.Year()), Count: 12},
		},
		Ratings: [10]int{0, 0, 0, 1, 0, 2, 1, 6, 2, 2},
		Genres: []model.LabelCount{
			{Label: "Fantasy", Count: 8}, {Label: "Classics", Count: 6},
			{Label: "Sci-Fi", Count: 4}, {Label: "Philosophy", Count: 3},
			{Label: "Nonfiction", Count: 2},
		},
		Heatmap: heatmap,
		Weekly:  stats,
	}

	return readingStats, sessions
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

func submitReviewRatingCmd(ctx context.Context, a *app.App, bookID int, rating float64, review string, seq int) tea.Cmd {
	return func() tea.Msg {
		if err := a.ReviewBook(ctx, bookID, rating, review); err != nil {
			return opDoneMsg{op: opReview, seq: seq, err: err}
		}
		info := fmt.Sprintf("Updated review and rating (%s)", model.StarString(rating))
		if strings.TrimSpace(review) == "" {
			info = fmt.Sprintf("Updated rating (%s)", model.StarString(rating))
		}
		return opDoneMsg{
			op:        opReview,
			seq:       seq,
			info:      info,
			reload:    true,
			markDirty: true,
		}

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
			info:    fmt.Sprintf("Session complete — %s", formatDuration(session.Duration())),
			session: session,
		}
	}
}

// ── Run ────────────────────────────────────────────────────────────────────

func runDashboard() error {
	a, err := initApp()
	if err != nil {
		return err
	}
	defer a.Store.Close()

	// The palette is adaptive; the config key only overrides what the
	// terminal reports about its background.
	if err := applyThemeSetting(a.Config.Theme); err != nil {
		return err
	}

	// Bubble Tea runs commands in goroutines it does not track, so cancel the
	// command context as soon as the program exits: in-flight API calls abort
	// instead of racing the store shutdown below.
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := tea.NewProgram(newDashboardModel(runCtx, a), tea.WithAltScreen())
	_, err = p.Run()
	cancel()
	return err

}

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive Oku dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDashboard()
		},
	}
}

func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}
