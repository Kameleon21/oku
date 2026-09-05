// Package tui is the Oku dashboard: the Bubble Tea model behind `oku tui`,
// the palette it draws with, and the sections it draws. It is imported by
// internal/cli and imports nothing from it, so the CLI commands and the
// dashboard share a theme without sharing a package.
package tui

import (
	"context"
	"fmt"
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
)

// ── Dashboard Model ────────────────────────────────────────────────────────

type Model struct {
	app *app.App
	// ctx is cancelled when the program exits so in-flight API calls abort
	// instead of outliving the store.
	ctx     context.Context
	version string

	// th is the palette, st the styles derived from it. Both are values on
	// the model so a future theme change can rebuild them mid-run.
	th Theme
	st styles

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
	density            Density

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

// New builds the dashboard model. density is the CLI's --view setting, which
// the list rows and the detail pane read to decide how much to show.
func New(ctx context.Context, a *app.App, density Density) Model {
	if ctx == nil {
		ctx = context.Background()
	}

	th := DefaultTheme()
	st := newStyles(th)

	delegate := newListDelegate(0, th)

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
	searchIn.PromptStyle = lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	searchIn.TextStyle = lipgloss.NewStyle().Foreground(th.Text)
	// Suggestions are the user's own search history, loaded with the rest of
	// the local data; there are none until they have searched for something.
	searchIn.ShowSuggestions = true

	pageIn := textinput.New()
	pageIn.Placeholder = "370 or +10 or -5"
	pageIn.CharLimit = 32
	pageIn.Prompt = "› "
	pageIn.PromptStyle = lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	pageIn.TextStyle = lipgloss.NewStyle().Foreground(th.Text)

	// The review fields sit inside a modal, so they carry its background: a
	// style that only sets a foreground would leave the field on the
	// terminal's own colour.
	reviewRatingIn := textinput.New()
	reviewRatingIn.Placeholder = "4.5"
	reviewRatingIn.CharLimit = 4
	reviewRatingIn.Prompt = reviewFieldFocused + "Rating: "
	reviewRatingIn.PromptStyle = st.modalKey
	reviewRatingIn.TextStyle = st.modalValue
	reviewRatingIn.PlaceholderStyle = st.modalDim

	reviewTextIn := textarea.New()
	reviewTextIn.Placeholder = "Write your review..."
	reviewTextIn.SetWidth(60)
	reviewTextIn.SetHeight(8)
	reviewTextIn.ShowLineNumbers = false
	for _, fs := range []*textarea.Style{&reviewTextIn.FocusedStyle, &reviewTextIn.BlurredStyle} {
		fs.Base = st.modalBg
		fs.Text = st.modalValue
		fs.Placeholder = st.modalDim
		fs.Prompt = st.modalKey
		fs.CursorLine = st.modalBg
		fs.EndOfBuffer = st.modalBg
	}

	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(th.Accent)

	hlp := help.New()
	hlp.ShortSeparator = " · "
	hlp.Styles.ShortKey = st.keyHint
	hlp.Styles.ShortDesc = st.desc
	hlp.Styles.ShortSeparator = st.dim
	hlp.Styles.Ellipsis = st.dim

	return Model{
		app:               a,
		ctx:               ctx,
		th:                th,
		st:                st,
		mode:              modeLibrary,
		section:           sectionReading,
		searchMode:        searchModeNormal,
		searchSub:         searchSubInput,
		searchQueryMode:   model.SearchModeBook,
		density:           density,
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

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spin.Tick,
		loadCachedLibraryCmd(m.app),
		loadLocalDataCmd(m.app),
		backgroundCheckCmd(),
	)
}

// ── Update ─────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
func (m Model) updateCommon(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			cmd := m.showToast(toastError, msg.err.Error())
			return m, cmd
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
			toastCmd = m.showToast(toastSuccess, msg.info)
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

func (m Model) applyLibraryLoaded(msg libraryLoadedMsg) (tea.Model, tea.Cmd) {
	m.endLoading()
	m.loaded = true
	if msg.reconcile {
		m.reconciling = false
	}
	if msg.err != nil {
		// dirty stays set: the local mutations are still unreconciled.
		cmd := m.showToast(toastError, msg.err.Error())
		return m, cmd
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

func (m Model) applySearchLoaded(msg searchLoadedMsg) (tea.Model, tea.Cmd) {
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
		cmd := m.showToast(toastError, msg.err.Error())
		return m, cmd
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
		toastCmd = m.showToast(toastSuccess, fmt.Sprintf("%s mode: loaded %d results", strings.ToLower(label), len(msg.results)))
	}
	saveCmd := m.addRecentSearchQuery(msg.query)
	m.updateSearchSuggestions()
	return m, tea.Batch(cmd, saveCmd, toastCmd)
}

// applyOpDone applies the result of a mutating operation. Results other than
// the review modal's own save leave the modal, and its draft, untouched.
func (m Model) applyOpDone(msg opDoneMsg) (tea.Model, tea.Cmd) {
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
func (m Model) applyReviewSaveDone(msg opDoneMsg) (tea.Model, tea.Cmd) {
	m.endLoading()
	m.reviewSubmitting = false

	if msg.err != nil {
		// Shown in the overlay; the rating and review text are preserved.
		m.reviewErr = msg.err.Error()
		return m, nil
	}
	toastCmd := m.showToast(toastSuccess, msg.info)
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

// startOp marks work as in flight and returns the model together with cmds and
// the spinner tick.
func (m Model) startOp(cmds ...tea.Cmd) (tea.Model, tea.Cmd) {
	cmd := m.beginLoading(cmds...)
	return m, cmd
}

// beginLoading counts one in-flight command per cmd and (re)starts the spinner
// tick loop. Each cmd must produce exactly one message whose handler calls
// endLoading.
func (m *Model) beginLoading(cmds ...tea.Cmd) tea.Cmd {
	for _, cmd := range cmds {
		if cmd != nil {
			m.inflight++
		}
	}
	return tea.Batch(append(cmds, m.startSpinner())...)
}

// endLoading records that one in-flight command has produced its result.
func (m *Model) endLoading() {
	if m.inflight > 0 {
		m.inflight--
	}
}

// isLoading reports whether any command is still running.
func (m Model) isLoading() bool {
	return m.inflight > 0
}

// startSpinner returns the spinner tick only when work is in flight and no
// loop is armed, so overlapping operations never run two of them at once.
func (m *Model) startSpinner() tea.Cmd {
	if m.spinning || (m.inflight == 0 && !m.searchLoading) {
		return nil
	}
	m.spinning = true
	return m.spin.Tick
}

// startTimerTick arms the one-second tick that refreshes the elapsed time,
// but only while a timer runs and no tick loop is armed yet.
func (m *Model) startTimerTick() tea.Cmd {
	if m.timerTicking || m.timerState == nil {
		return nil
	}
	m.timerTicking = true
	return timerTickCmd()
}

func (m Model) updateLibraryMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

func (m Model) handleGenericKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

// ── View ───────────────────────────────────────────────────────────────────

func (m Model) View() string {
	return m.fitToScreen(m.frame())
}

// frame renders the dashboard at its natural size. View clamps it to the
// terminal; keeping the two apart lets a test assert that the layout really
// fills the screen instead of being padded into it.
func (m Model) frame() string {
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
		return m.overlayModal(renderConfirmModal(m.confirm, max(36, min(60, m.width-10)), m.st))
	}
	if m.showHelp {
		return m.overlayModal(m.renderHelpModal())
	}
	return body
}

// statusBar renders the top bar: the app name and spinner on the left, the
// latest message on the right, over an unbroken background.
func (m Model) statusBar() string {
	width := max(20, m.width)
	inner := max(1, width-m.st.statusBar.GetHorizontalPadding())

	left := m.st.statusBarTitle.Render("oku")
	if m.isLoading() {
		left += m.st.statusBarAccent.Render(" " + m.spin.View())
	}

	right := m.renderToast(inner - lipgloss.Width(left) - 2)

	gap := max(1, inner-lipgloss.Width(left)-lipgloss.Width(right))
	return m.st.statusBar.Width(width).Render(
		left + m.st.statusBarFill.Render(strings.Repeat(" ", gap)) + right,
	)
}

// ── Run ────────────────────────────────────────────────────────────────────

// Run starts the dashboard on a, with density as the row detail the CLI's
// --view flag asked for. It returns when the user quits.
func Run(ctx context.Context, a *app.App, density Density) error {
	// Bubble Tea runs commands in goroutines it does not track, so cancel the
	// command context as soon as the program exits: in-flight API calls abort
	// instead of racing the caller's store shutdown.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	p := tea.NewProgram(New(runCtx, a, density), tea.WithAltScreen())
	_, err := p.Run()
	cancel()
	return err
}
