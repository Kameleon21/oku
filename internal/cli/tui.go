package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	err          error
}

type searchLoadedMsg struct {
	results []model.SearchResult
	query   string
	mode    model.SearchMode
	err     error
}

type opDoneMsg struct {
	info      string
	err       error
	reload    bool
	markDirty bool
}

type backgroundCheckMsg struct{}

type timerTickMsg time.Time

type localDataLoadedMsg struct {
	readingStats   *model.ReadingStats
	recentSessions []model.ReadingSession
	timerState     *model.TimerState
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
	loading bool

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

	readingBooks []model.UserBook
	okuBooks     []model.UserBook
	searchBooks  []model.SearchResult

	pendingBookID    int
	reviewBook       *model.UserBook
	reviewFocus      dashboardReviewFocus
	reviewSubmitting bool
	dirty            bool
	lastMutationAt   time.Time

	lastQuery      string
	lastSearchMode model.SearchMode
	infoMsg        string
	errMsg         string

	searchLoading      bool
	searchLoadingQuery string
	searchMode         searchInputMode
	searchSub          searchSubFocus
	searchQueryMode    model.SearchMode
	recentSearches     []string
	density            outputDensity

	showHelp bool

	// Local data for stats/timer sections.
	timerState     *model.TimerState
	readingStats   *model.ReadingStats
	weeklyStats    model.WeeklyStats
	recentSessions []model.ReadingSession
	localLoaded    bool
	statsScroll    int

	timerSelecting bool
	timerSelectIdx int
}

func newDashboardModel(ctx context.Context, a *app.App) dashboardModel {
	if ctx == nil {
		ctx = context.Background()
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(0)

	delegate.Styles.NormalTitle = lipgloss.NewStyle().
		Foreground(colorLightGray).
		Padding(0, 0, 0, 2)
	delegate.Styles.NormalDesc = lipgloss.NewStyle().
		Foreground(colorMidGray).
		Padding(0, 0, 0, 2)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(colorGold).
		Foreground(colorGold).
		Bold(true).
		Padding(0, 0, 0, 1)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(colorGold).
		Foreground(colorMidGray).
		Padding(0, 0, 0, 1)
	delegate.Styles.DimmedTitle = lipgloss.NewStyle().
		Foreground(colorDimGray).
		Padding(0, 0, 0, 2)
	delegate.Styles.DimmedDesc = lipgloss.NewStyle().
		Foreground(colorDarkGray).
		Padding(0, 0, 0, 2)

	newList := func(title string) list.Model {
		l := list.New(nil, delegate, 40, 12)
		l.Title = title
		l.SetShowStatusBar(false)
		l.SetFilteringEnabled(true)
		l.SetShowHelp(false)
		l.DisableQuitKeybindings()

		styles := list.DefaultStyles()
		styles.Title = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCream).
			Background(colorCharcoal).
			Padding(0, 1)
		styles.TitleBar = lipgloss.NewStyle().Padding(0, 0, 1, 0)
		l.Styles = styles
		return l
	}

	searchIn := textinput.New()
	searchIn.Placeholder = "Search books..."
	searchIn.CharLimit = 120
	searchIn.Prompt = "/ "
	searchIn.PromptStyle = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	searchIn.TextStyle = lipgloss.NewStyle().Foreground(colorLightGray)
	searchIn.ShowSuggestions = true
	searchIn.SetSuggestions(defaultSearchSuggestions(model.SearchModeBook))

	pageIn := textinput.New()
	pageIn.Placeholder = "370 or +10 or -5"
	pageIn.CharLimit = 32
	pageIn.Prompt = "› "
	pageIn.PromptStyle = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	pageIn.TextStyle = lipgloss.NewStyle().Foreground(colorLightGray)

	reviewRatingIn := textinput.New()
	reviewRatingIn.Placeholder = "4.5"
	reviewRatingIn.CharLimit = 4
	reviewRatingIn.Prompt = "Rating: "
	reviewRatingIn.PromptStyle = lipgloss.NewStyle().Foreground(colorGold).Bold(true)
	reviewRatingIn.TextStyle = lipgloss.NewStyle().Foreground(colorLightGray)

	reviewTextIn := textarea.New()
	reviewTextIn.Placeholder = "Write your review..."
	reviewTextIn.SetWidth(60)
	reviewTextIn.SetHeight(8)
	reviewTextIn.ShowLineNumbers = false

	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(colorGold)

	return dashboardModel{
		app:               a,
		ctx:               ctx,
		mode:              modeLibrary,
		section:           sectionReading,
		searchMode:        searchModeNormal,
		searchSub:         searchSubInput,
		searchQueryMode:   model.SearchModeBook,
		density:           currentOutputDensity(),
		readingList:       newList("Reading"),
		okuList:           newList("Oku"),
		searchList:        newList("Search Results"),
		searchInput:       searchIn,
		pageInput:         pageIn,
		reviewRatingInput: reviewRatingIn,
		reviewTextInput:   reviewTextIn,
		spin:              s,
	}
}

// ── Init ───────────────────────────────────────────────────────────────────

func (m dashboardModel) Init() tea.Cmd {
	return tea.Batch(
		m.spin.Tick,
		loadCachedLibraryCmd(m.app),
		loadLocalDataCmd(m.app),
		backgroundCheckCmd(),
		timerTickCmd(),
	)
}

// ── Update ─────────────────────────────────────────────────────────────────

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeUpdatePage:
		return m.updatePageMode(msg)
	case modeReviewRating:
		return m.updateReviewRatingMode(msg)
	default:
		return m.updateLibraryMode(msg)
	}
}

func (m dashboardModel) updateLibraryMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case timerTickMsg:
		// Just triggers a re-render for the timer display.
		return m, timerTickCmd()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case libraryLoadedMsg:
		m.loading = false
		m.loaded = true
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.infoMsg = ""
			return m, nil
		}
		m.readingBooks = msg.reading
		m.okuBooks = msg.oku
		if m.timerSelectIdx >= len(m.readingBooks) {
			m.timerSelectIdx = max(0, len(m.readingBooks)-1)
		}
		m.refreshListItems()
		m.updateSearchSuggestions()
		m.errMsg = ""
		if msg.needsRefresh {
			m.loading = true
			return m, loadLibraryCmd(m.ctx, m.app, true)
		}
		return m, nil

	case localDataLoadedMsg:
		m.localLoaded = true
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.readingStats = msg.readingStats
		if msg.readingStats != nil {
			m.weeklyStats = msg.readingStats.Weekly
		}
		m.recentSessions = msg.recentSessions
		m.timerState = msg.timerState
		return m, nil

	case timerOpDoneMsg:
		m.timerSelecting = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.infoMsg = ""
		} else {
			m.errMsg = ""
			m.infoMsg = msg.info
		}
		// Reload local data after timer operations.
		return m, loadLocalDataCmd(m.app)

	case searchLoadedMsg:
		m.loading = false
		m.searchLoading = false
		m.searchLoadingQuery = ""
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.infoMsg = ""
			return m, nil
		}
		m.searchQueryMode = msg.mode
		m.lastQuery = msg.query
		m.lastSearchMode = msg.mode
		m.searchBooks = msg.results
		m.refreshSearchResultItems()
		m.searchList.Title = fmt.Sprintf("%s Results (%d)", m.searchQueryMode.Label(), len(msg.results))
		if len(msg.results) > 0 {
			m.searchSub = searchSubResults
		}
		m.errMsg = ""
		if len(msg.results) == 0 {
			m.infoMsg = fmt.Sprintf("%s mode: no results for %q", strings.ToLower(m.searchQueryMode.Label()), msg.query)
		} else {
			m.infoMsg = fmt.Sprintf("%s mode: loaded %d results", strings.ToLower(m.searchQueryMode.Label()), len(msg.results))
		}
		m.addRecentSearchQuery(msg.query)
		m.updateSearchSuggestions()
		return m, nil

	case backgroundCheckMsg:
		if m.dirty && !m.loading && time.Since(m.lastMutationAt) >= backgroundSyncWindow {
			m.loading = true
			m.dirty = false
			return m, tea.Batch(loadLibraryCmd(m.ctx, m.app, true), backgroundCheckCmd())
		}
		return m, backgroundCheckCmd()

	case opDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.infoMsg = ""
		} else {
			m.errMsg = ""
			m.infoMsg = msg.info
			if msg.markDirty {
				m.dirty = true
				m.lastMutationAt = time.Now()
			}
		}
		if msg.reload {
			m.loading = true
			return m, tea.Batch(loadLibraryCmd(m.ctx, m.app, false), loadLocalDataCmd(m.app))
		}
		return m, nil

	case tea.KeyMsg:
		// Help modal intercepts all keys.
		if m.showHelp {
			switch msg.String() {
			case "?", "esc":
				m.showHelp = false
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
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

	// Forward to active list for scroll/filter.
	var cmd tea.Cmd
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

// ── Section-specific key handlers ──────────────────────────────────────────

func (m dashboardModel) handleGenericKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "j", "down", "l", "right", "tab":
		m.nextSection()
		return m, nil
	case "k", "up", "h", "left", "shift+tab":
		m.prevSection()
		return m, nil
	case "/":
		m.section = sectionSearch
		m.searchSub = searchSubInput
		m.enterSearchInsertMode()
		m.searchInput.CursorEnd()
		m.updateSearchSuggestions()
		m.errMsg = ""
		return m, nil
	}
	return m, nil
}

func (m dashboardModel) handleStatsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		content := m.statsView(m.rightPanelContentWidth())
		_, m.statsScroll = clipLines(content, m.statsScroll+1, m.rightPanelContentHeight())
		return m, nil
	case "k", "up":
		if m.statsScroll > 0 {
			m.statsScroll--
		}
		return m, nil
	case "g":
		m.statsScroll = 0
		return m, nil
	case "r":
		return m, loadLocalDataCmd(m.app)
	case "s":
		m.loading = true
		return m, syncAllAndReloadCmd(m.ctx, m.app)
	case "l", "right", "tab":
		m.statsScroll = 0
		m.nextSection()
		return m, nil
	case "h", "left", "shift+tab":
		m.statsScroll = 0
		m.prevSection()
		return m, nil
	}
	return m.handleGenericKeys(msg)
}

func (m dashboardModel) handleLibraryKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "l", "right", "tab":
		m.nextSection()
		return m, nil
	case "h", "left", "shift+tab":
		m.prevSection()
		return m, nil
	case "j", "down":
		// If list has focus, forward to list for item navigation.
		var cmd tea.Cmd
		if m.section == sectionReading {
			m.readingList, cmd = m.readingList.Update(msg)
		} else {
			m.okuList, cmd = m.okuList.Update(msg)
		}
		return m, cmd
	case "k", "up":
		var cmd tea.Cmd
		if m.section == sectionReading {
			m.readingList, cmd = m.readingList.Update(msg)
		} else {
			m.okuList, cmd = m.okuList.Update(msg)
		}
		return m, cmd
	case "/":
		m.section = sectionSearch
		m.searchSub = searchSubInput
		m.enterSearchInsertMode()
		m.searchInput.CursorEnd()
		m.updateSearchSuggestions()
		m.errMsg = ""
		return m, nil
	case "r":
		m.loading = true
		return m, loadLibraryCmd(m.ctx, m.app, true)
	case "s":
		m.loading = true
		return m, syncAllAndReloadCmd(m.ctx, m.app)
	case "z":
		m.cycleDensity()
		return m, nil
	case "enter":
		if m.section == sectionOku {
			return m.changeSelectedLibraryStatus(model.StatusCurrentlyReading)
		}
		return m.changeSelectedLibraryStatus(model.StatusWantToRead)
	case "+", "=":
		if b := m.selectedLibraryBook(); b != nil {
			m.loading = true
			return m, quickProgressCmd(m.ctx, m.app, b.Book.ID, +10)
		}
	case "-":
		if b := m.selectedLibraryBook(); b != nil {
			m.loading = true
			return m, quickProgressCmd(m.ctx, m.app, b.Book.ID, -10)
		}
	case "u":
		if b := m.selectedLibraryBook(); b != nil {
			m.mode = modeUpdatePage
			m.pendingBookID = b.Book.ID
			m.pageInput.SetValue("")
			m.pageInput.Placeholder = fmt.Sprintf("Update %s", b.Book.Title)
			m.pageInput.Focus()
			return m, nil
		}
	case "v":
		if b := m.selectedLibraryBook(); b != nil {
			m.openReviewRatingModal(*b)
			return m, nil
		}
	case "g":
		return m.changeSelectedLibraryStatus(model.StatusCurrentlyReading)
	case "w":
		return m.changeSelectedLibraryStatus(model.StatusWantToRead)
	case "f":
		return m.changeSelectedLibraryStatus(model.StatusRead)
	case "d":
		return m.changeSelectedLibraryStatus(model.StatusDidNotFinish)
	case "x":
		return m.changeSelectedLibraryStatus(model.StatusIgnored)
	}
	return m, nil
}

func (m dashboardModel) handleSearchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searchSub == searchSubInput {
		if m.searchMode == searchModeNormal {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "?":
				m.showHelp = true
				return m, nil
			case "i":
				m.enterSearchInsertMode()
				return m, nil
			case "a":
				m.enterSearchInsertMode()
				m.searchInput.CursorEnd()
				return m, nil
			case "m":
				m.setSearchQueryMode(m.searchQueryMode.Next())
				return m, nil
			case "1":
				m.setSearchQueryMode(model.SearchModeBook)
				return m, nil
			case "2":
				m.setSearchQueryMode(model.SearchModeAuthor)
				return m, nil
			case "3":
				m.setSearchQueryMode(model.SearchModeGenre)
				return m, nil
			case "z":
				m.cycleDensity()
				return m, nil
			case "enter":
				return m, m.submitSearch()
			case "l", "right", "tab":
				m.nextSection()
				m.searchInput.Blur()
				return m, nil
			case "esc", "h", "left", "shift+tab":
				m.prevSection()
				m.searchInput.Blur()
				return m, nil
			case "j", "down":
				if m.hasSearchResults() {
					m.searchSub = searchSubResults
				} else {
					m.nextSection()
				}
				return m, nil
			case "k", "up":
				m.prevSection()
				m.searchInput.Blur()
				return m, nil
			}
			return m, nil
		}

		// Insert mode.
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = true
			return m, nil
		case "enter":
			return m, m.submitSearch()
		case "esc":
			m.enterSearchNormalMode()
			return m, nil
		}

		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	// searchSubResults
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "esc", "h", "left":
		m.searchSub = searchSubInput
		m.enterSearchNormalMode()
		return m, nil
	case "l", "right", "tab":
		m.nextSection()
		return m, nil
	case "shift+tab":
		m.prevSection()
		return m, nil
	case "enter":
		if r := m.selectedSearchResult(); r != nil {
			m.loading = true
			return m, addFromSearchCmd(m.ctx, m.app, r.ID, model.StatusCurrentlyReading)
		}
	case "g":
		return m.changeSelectedSearchStatus(model.StatusCurrentlyReading)
	case "w":
		return m.changeSelectedSearchStatus(model.StatusWantToRead)
	case "f":
		return m.changeSelectedSearchStatus(model.StatusRead)
	case "d":
		return m.changeSelectedSearchStatus(model.StatusDidNotFinish)
	case "z":
		m.cycleDensity()
		return m, nil
	case "j", "down", "k", "up":
		var cmd tea.Cmd
		m.searchList, cmd = m.searchList.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.searchList, cmd = m.searchList.Update(msg)
	return m, cmd
}

func (m dashboardModel) handleTimerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.timerSelecting && m.timerState == nil {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = true
			return m, nil
		case "esc":
			m.timerSelecting = false
			m.infoMsg = "Timer start cancelled"
			return m, nil
		case "j", "down":
			if m.timerSelectIdx < len(m.readingBooks)-1 {
				m.timerSelectIdx++
			}
			return m, nil
		case "k", "up":
			if m.timerSelectIdx > 0 {
				m.timerSelectIdx--
			}
			return m, nil
		case "enter":
			if len(m.readingBooks) == 0 {
				m.timerSelecting = false
				m.errMsg = "no currently reading books available"
				m.infoMsg = ""
				return m, nil
			}
			// Background sync can shrink readingBooks while the picker is open.
			if m.timerSelectIdx >= len(m.readingBooks) {
				m.timerSelectIdx = len(m.readingBooks) - 1
			}
			selected := m.readingBooks[m.timerSelectIdx]
			m.loading = true
			m.timerSelecting = false
			return m, startTimerForBookCmd(m.app, selected.Book.ID)
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "j", "down", "l", "right", "tab":
		m.nextSection()
		return m, nil
	case "k", "up", "h", "left", "shift+tab":
		m.prevSection()
		return m, nil
	case "/":
		m.section = sectionSearch
		m.searchSub = searchSubInput
		m.enterSearchInsertMode()
		m.searchInput.CursorEnd()
		m.updateSearchSuggestions()
		return m, nil
	case "t":
		if m.timerState == nil {
			if len(m.readingBooks) == 0 {
				m.errMsg = "no currently reading books available"
				m.infoMsg = "Add a book to Reading, then start a timer."
				return m, nil
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
			m.errMsg = ""
			m.infoMsg = "Select a book and press Enter to start timer"
			return m, nil
		}
	case "s":
		if m.timerState != nil {
			return m, stopTimerCmd(m.app)
		}
	}
	return m, nil
}

// ── Page update mode ───────────────────────────────────────────────────────

func (m dashboardModel) updatePageMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case timerTickMsg:
		return m, timerTickCmd()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case backgroundCheckMsg:
		if m.dirty && !m.loading && time.Since(m.lastMutationAt) >= backgroundSyncWindow {
			m.loading = true
			m.dirty = false
			return m, tea.Batch(loadLibraryCmd(m.ctx, m.app, true), backgroundCheckCmd())
		}
		return m, backgroundCheckCmd()
	case opDoneMsg:
		m.loading = false
		m.mode = modeLibrary
		m.pageInput.Blur()
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.infoMsg = ""
		} else {
			m.errMsg = ""
			m.infoMsg = msg.info
			if msg.markDirty {
				m.dirty = true
				m.lastMutationAt = time.Now()
			}
		}
		if msg.reload {
			m.loading = true
			return m, tea.Batch(loadLibraryCmd(m.ctx, m.app, false), loadLocalDataCmd(m.app))
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.mode = modeLibrary
			m.pageInput.Blur()
			m.pageInput.SetValue("")
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			raw := strings.TrimSpace(m.pageInput.Value())
			if raw == "" {
				m.errMsg = "page value cannot be empty"
				return m, nil
			}
			m.loading = true
			return m, updateProgressCmd(m.ctx, m.app, m.pendingBookID, raw)
		}
	}

	var cmd tea.Cmd
	m.pageInput, cmd = m.pageInput.Update(msg)
	return m, cmd
}

// ── Review/rating mode ─────────────────────────────────────────────────────

func (m dashboardModel) updateReviewRatingMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case timerTickMsg:
		return m, timerTickCmd()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		textareaWidth := max(40, m.width/2)
		m.reviewTextInput.SetWidth(textareaWidth)
		return m, nil
	case backgroundCheckMsg:
		if m.dirty && !m.loading && time.Since(m.lastMutationAt) >= backgroundSyncWindow {
			m.loading = true
			m.dirty = false
			return m, tea.Batch(loadLibraryCmd(m.ctx, m.app, true), backgroundCheckCmd())
		}
		return m, backgroundCheckCmd()
	case opDoneMsg:
		m.loading = false
		m.reviewSubmitting = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.infoMsg = ""
			return m, nil
		}

		m.errMsg = ""
		m.infoMsg = msg.info
		if msg.markDirty {
			m.dirty = true
			m.lastMutationAt = time.Now()
		}
		m.closeReviewRatingModal()
		if msg.reload {
			m.loading = true
			return m, loadLibraryCmd(m.ctx, m.app, false)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.closeReviewRatingModal()
			m.infoMsg = "Review update cancelled"
			return m, nil
		case "tab":
			if m.reviewFocus == dashboardReviewFocusRating {
				m.focusReviewTextField()
			} else {
				m.focusReviewRatingField()
			}
			return m, nil
		case "shift+tab":
			if m.reviewFocus == dashboardReviewFocusText {
				m.focusReviewRatingField()
			} else {
				m.focusReviewTextField()
			}
			return m, nil
		case "ctrl+s":
			if m.reviewSubmitting || m.reviewBook == nil {
				return m, nil
			}
			rating, err := parseReviewRatingInput(m.reviewRatingInput.Value())
			if err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
			m.errMsg = ""
			m.loading = true
			m.reviewSubmitting = true
			review := m.reviewTextInput.Value()
			bookID := m.reviewBook.Book.ID
			m.infoMsg = reviewSavePendingMessage(review)
			m.closeReviewRatingModal()
			return m, submitReviewRatingCmd(m.ctx, m.app, bookID, rating, review)
		}
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
	if !m.loaded {
		return "\n  " + m.spin.View() + " Loading dashboard..."
	}

	// Status bar.
	left := titleBarStyle.Render(" oku")
	if m.loading {
		left += " " + m.spin.View()
	}

	rightContent := ""
	if m.errMsg != "" {
		rightContent = errorStyleTUI.Render(m.errMsg)
	} else if m.infoMsg != "" {
		rightContent = infoStyleTUI.Render(m.infoMsg)
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(rightContent) - 4
	if gap < 1 {
		gap = 1
	}
	statusBar := statusBarStyle.Width(max(60, m.width)).Render(
		left + strings.Repeat(" ", gap) + rightContent,
	)

	// Body.
	var body string
	switch m.mode {
	case modeUpdatePage:
		pagePrompt := "\n " + keyStyle.Render("Page update") + " " + m.pageInput.View() +
			dimStyleTUI.Render("  (Enter submit, Esc cancel)")
		body = statusBar + "\n" + m.renderLayout() + pagePrompt
	default:
		body = statusBar + "\n" + m.renderLayout() + "\n" + m.contextHelpBar()
	}

	if m.mode == modeReviewRating {
		return m.overlayModal(m.reviewRatingOverlay())
	}
	if m.showHelp {
		return m.overlayModal(m.renderHelpModal())
	}
	return body
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
	totalH := max(8, m.height-6)
	return max(1, totalH-2)
}

// renderLayout renders the 2-column layout: left sections + right context panel.
func (m dashboardModel) renderLayout() string {
	totalW := max(60, m.width-2)
	totalH := max(8, m.height-6)
	panelInnerH := max(1, totalH-2)
	leftW := max(28, totalW*2/5)

	leftContent := m.renderSections(leftW-2, panelInnerH)
	leftPanel := panelFocusedStyle.Width(leftW).Height(panelInnerH).Render(leftContent)

	// Right panel: context-sensitive.
	rightW := max(28, m.width-lipgloss.Width(leftPanel)-2)
	rightContent := m.rightPanelView(rightW - 4)
	rightPanel := panelStyle.
		Width(rightW).
		Height(panelInnerH).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
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
	reduceOrder := []focusSection{
		sectionStats, sectionTimer, sectionIntro, sectionSearch,
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
	label := m.formatSectionLabel(def.id, def.label, def.count, focused)
	if h <= 0 {
		return ""
	}
	if h < 3 {
		return lipgloss.NewStyle().Width(w).Render(label)
	}
	innerH := h - 2

	content := label
	if def.id == sectionReading || def.id == sectionOku || def.id == sectionSearch {
		contentH := innerH - 1 // label + remaining content area
		if contentH < 0 {
			contentH = 0
		}
		if contentH > 0 {
			if body := m.sectionContent(def.id, max(8, w-4), contentH); body != "" {
				content += "\n" + body
			}
		}
	}

	style := panelStyle
	if focused {
		style = panelFocusedStyle
	}
	return style.Width(w).Height(innerH).Render(content)
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
func (m dashboardModel) sectionContent(id focusSection, w, h int) string {
	switch id {
	case sectionReading:
		m.readingList.SetSize(w, h)
		return m.readingList.View()
	case sectionOku:
		m.okuList.SetSize(w, h)
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
		return m.detailsView()
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

func (m dashboardModel) detailsView() string {
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
		progressText += "  " + progressBar(page, b.Book.Pages, 20)
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

	return m.searchList.View()
}

func (m *dashboardModel) openReviewRatingModal(book model.UserBook) {
	b := book
	m.reviewBook = &b
	m.mode = modeReviewRating
	m.showHelp = false
	m.reviewSubmitting = false

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
	m.reviewRatingInput.Blur()
	m.reviewTextInput.Blur()
}

func (m *dashboardModel) focusReviewRatingField() {
	m.reviewFocus = dashboardReviewFocusRating
	m.reviewRatingInput.Focus()
	m.reviewTextInput.Blur()
}

func (m *dashboardModel) focusReviewTextField() {
	m.reviewFocus = dashboardReviewFocusText
	m.reviewRatingInput.Blur()
	m.reviewTextInput.Focus()
}

func (m dashboardModel) reviewRatingOverlay() string {
	if m.reviewBook == nil {
		return renderModalPanel("Review / Rate Book", "No book selected", 48)
	}

	rating, err := parseReviewRatingInput(m.reviewRatingInput.Value())
	ratingLabel := "invalid rating"
	stars := "☆☆☆☆☆"
	if err == nil {
		stars = model.StarString(rating)
		ratingLabel = fmt.Sprintf("%.1f", rating)
	}

	var sb strings.Builder
	sb.WriteString(valueStyle.Render(m.reviewBook.Book.Title))
	sb.WriteString("\n")
	sb.WriteString(dimStyleTUI.Render(fallback(m.reviewBook.Book.AuthorString(), "Unknown author")))
	sb.WriteString("\n\n")
	sb.WriteString(m.reviewRatingInput.View())
	sb.WriteString("  ")
	sb.WriteString(keyStyle.Render(stars))
	sb.WriteString(" ")
	sb.WriteString(dimStyleTUI.Render("(" + ratingLabel + ")"))
	sb.WriteString("\n\n")
	sb.WriteString(labelStyle.Render("Review"))
	sb.WriteString("\n")
	sb.WriteString(m.reviewTextInput.View())
	sb.WriteString("\n\n")
	if m.reviewSubmitting {
		sb.WriteString(dimStyleTUI.Render("Saving..."))
		sb.WriteString("\n")
	}
	sb.WriteString(dimStyleTUI.Render("Tab/Shift+Tab switch fields   Ctrl+S save   Esc cancel"))

	width := max(70, m.width-10)
	if width > 100 {
		width = 100
	}
	return renderModalPanel("Review / Rate Book", sb.String(), width)
}

// ── Help ───────────────────────────────────────────────────────────────────

func (m dashboardModel) renderHelpModal() string {
	section := func(title string, keys [][2]string) string {
		s := headStyle.Render(title) + "\n"
		for _, k := range keys {
			s += fmt.Sprintf("  %s  %s\n",
				keyStyle.Width(12).Render(k[0]),
				descStyle.Render(k[1]),
			)
		}
		return s
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		section("Navigation", [][2]string{
			{"j / k", "Move up / down in list"},
			{"h / l", "Move section left / right"},
			{"Tab", "Next section (alias)"},
			{"Shift+Tab", "Previous section (alias)"},
			{"/", "Focus search input"},
			{"Esc", "Back / cancel"},
		}),
		section("Library Actions", [][2]string{
			{"Enter", "Toggle reading / oku"},
			{"+ / -", "Quick page update (+/-10)"},
			{"u", "Update page progress"},
			{"v", "Review / rate book"},
			{"g", "Set status: reading"},
			{"w", "Set status: want to read"},
			{"f", "Set status: finished"},
			{"d", "Set status: did not finish"},
			{"x", "Remove from library"},
			{"z", "Cycle density"},
		}),
		section("Data", [][2]string{
			{"r", "Refresh from cache"},
			{"s", "Full sync with Hardcover"},
		}),
		section("Search", [][2]string{
			{"Enter", "Execute search / add book"},
			{"i / a", "Enter insert mode"},
			{"m", "Cycle mode: book/author/genre"},
			{"1/2/3", "Set mode: book/author/genre"},
			{"g/w/f/d", "Add result with status"},
		}),
		section("Timer", [][2]string{
			{"t", "Choose book + start"},
			{"s", "Stop timer"},
			{"j/k + Enter", "Pick book in timer"},
			{"Esc", "Cancel timer picker"},
		}),
		section("General", [][2]string{
			{"?", "Toggle this help"},
			{"q", "Quit"},
		}),
	)

	footer := "\n" + dimStyleTUI.Render("Press ? or Esc to close")
	return renderModalPanel("Help", body+footer, 50)
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

func (m dashboardModel) contextHelpBar() string {
	switch m.section {
	case sectionIntro:
		return renderHelpBar([][2]string{
			{"h/l", "section"},
			{"Tab", "next"},
			{"/", "search"},
			{"?", "help"},
			{"q", "quit"},
		})
	case sectionReading, sectionOku:
		return renderHelpBar([][2]string{
			{"j/k", "navigate"},
			{"h/l", "section"},
			{"/", "search"},
			{"↵", "toggle"},
			{"+/-", "page"},
			{"u", "update"},
			{"v", "review/rate"},
			{"g/w/f/d", "status"},
			{"z", "density"},
			{"r", "refresh"},
			{"s", "sync"},
			{"?", "help"},
		})
	case sectionSearch:
		if m.searchSub == searchSubResults {
			return renderHelpBar([][2]string{
				{"j/k", "navigate"},
				{"↵", "add reading"},
				{"g/w/f/d", "status"},
				{"z", "density"},
				{"h/l", "input/next"},
				{"Esc", "back"},
				{"?", "help"},
			})
		}
		if m.searchMode == searchModeInsert {
			return renderHelpBar([][2]string{
				{"↵", "search"},
				{"Esc", "normal"},
				{"?", "help"},
			})
		}
		return renderHelpBar([][2]string{
			{"↵", "search"},
			{"i/a", "insert"},
			{"m", "mode"},
			{"1/2/3", "book/author/genre"},
			{"h/l", "section"},
			{"Esc", "back"},
			{"?", "help"},
		})
	case sectionStats:
		return renderHelpBar([][2]string{
			{"j/k", "scroll"},
			{"g", "top"},
			{"h/l", "section"},
			{"s", "sync"},
			{"/", "search"},
			{"?", "help"},
			{"q", "quit"},
		})
	case sectionTimer:
		if m.timerSelecting && m.timerState == nil {
			return renderHelpBar([][2]string{
				{"j/k", "choose"},
				{"↵", "start"},
				{"Esc", "cancel"},
				{"?", "help"},
				{"q", "quit"},
			})
		}
		if m.timerState != nil {
			return renderHelpBar([][2]string{
				{"s", "stop"},
				{"h/l", "section"},
				{"/", "search"},
				{"?", "help"},
				{"q", "quit"},
			})
		}
		return renderHelpBar([][2]string{
			{"t", "choose + start"},
			{"h/l", "section"},
			{"/", "search"},
			{"?", "help"},
			{"q", "quit"},
		})
	default:
		return renderHelpBar([][2]string{
			{"h/l", "section"},
			{"Tab", "next"},
			{"/", "search"},
			{"?", "help"},
			{"q", "quit"},
		})
	}
}

// ── Navigation helpers ─────────────────────────────────────────────────────

func (m *dashboardModel) nextSection() {
	m.searchInput.Blur()
	m.section = (m.section + 1) % sectionCount
}

func (m *dashboardModel) prevSection() {
	m.searchInput.Blur()
	m.section = (m.section - 1 + sectionCount) % sectionCount
}

// ── Search helpers ─────────────────────────────────────────────────────────

func (m dashboardModel) hasSearchResults() bool {
	return len(m.searchList.Items()) > 0
}

func (m *dashboardModel) submitSearch() tea.Cmd {
	if m.searchLoading {
		return nil
	}

	query := strings.TrimSpace(m.searchInput.Value())
	if query == "" {
		m.errMsg = "search query cannot be empty"
		return nil
	}

	// Reuse in-memory results for the same query instead of refetching.
	if strings.EqualFold(query, strings.TrimSpace(m.lastQuery)) &&
		m.searchQueryMode == m.lastSearchMode && len(m.searchList.Items()) > 0 {
		m.searchSub = searchSubResults
		m.searchMode = searchModeNormal
		m.searchInput.Blur()
		m.errMsg = ""
		m.infoMsg = fmt.Sprintf("%s mode: showing cached results for %q",
			strings.ToLower(m.searchQueryMode.Label()),
			query,
		)
		return nil
	}

	m.loading = true
	m.searchLoading = true
	m.searchLoadingQuery = query
	m.errMsg = ""
	m.infoMsg = fmt.Sprintf("%s mode (%s): searching for %q...",
		strings.ToLower(m.searchQueryMode.Label()),
		m.searchQueryMode.Description(),
		query,
	)
	m.searchList.Title = fmt.Sprintf("%s Results (loading...)", m.searchQueryMode.Label())
	return searchCmd(m.ctx, m.app, query, m.searchQueryMode)
}

func (m *dashboardModel) cycleDensity() {
	switch m.density {
	case densityCompact:
		m.density = densityDefault
	case densityDefault:
		m.density = densityVerbose
	default:
		m.density = densityCompact
	}
	m.refreshListItems()
	m.refreshSearchResultItems()
	m.infoMsg = "Density: " + densityLabel(m.density)
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

func (m *dashboardModel) setSearchQueryMode(mode model.SearchMode) {
	if mode == "" {
		mode = model.SearchModeBook
	}
	if m.searchQueryMode == mode {
		return
	}
	m.searchQueryMode = mode
	m.searchList.Title = fmt.Sprintf("%s Results", mode.Label())
	m.infoMsg = fmt.Sprintf("Search mode: %s (%s)", strings.ToLower(mode.Label()), mode.Description())
	m.searchInput.Placeholder = searchPlaceholderForMode(mode)
	m.updateSearchSuggestions()
}

func (m *dashboardModel) addRecentSearchQuery(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}
	next := []string{query}
	for _, q := range m.recentSearches {
		if strings.EqualFold(q, query) {
			continue
		}
		next = append(next, q)
		if len(next) >= 8 {
			break
		}
	}
	m.recentSearches = next
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

	for _, s := range defaultSearchSuggestions(m.searchQueryMode) {
		push(s)
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

func defaultSearchSuggestions(mode model.SearchMode) []string {
	switch mode {
	case model.SearchModeAuthor:
		return []string{
			"Ursula K. Le Guin",
			"Octavia Butler",
			"Neal Ford",
			"Fyodor Dostoevsky",
			"James Clear",
		}
	case model.SearchModeGenre:
		return []string{
			"science fiction",
			"classics",
			"philosophy",
			"software architecture",
			"psychology",
		}
	default:
		return []string{
			"east of eden",
			"dune",
			"atomic habits",
			"clean code",
			"the brothers karamazov",
		}
	}
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
	totalW := max(60, m.width-2)
	totalH := max(8, m.height-6)
	panelInnerH := max(1, totalH-2)

	leftW := max(28, totalW*2/5)
	rightW := max(28, totalW-leftW-3)

	heights := m.leftSectionHeights(panelInnerH)
	readingContentH := max(1, heights[sectionReading]-3)
	okuContentH := max(1, heights[sectionOku]-3)
	leftContentW := leftW - 6
	if leftContentW < 8 {
		leftContentW = 8
	}

	m.readingList.SetSize(leftContentW, readingContentH)
	m.okuList.SetSize(leftContentW, okuContentH)
	m.searchList.SetSize(rightW-4, max(1, panelInnerH-2))
}

// ── List helpers ───────────────────────────────────────────────────────────

func (m *dashboardModel) refreshListItems() {
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
	m.readingList.SetItems(toItems(m.readingBooks))
	m.okuList.SetItems(toItems(m.okuBooks))
	m.readingList.Title = fmt.Sprintf("Reading (%d)", len(m.readingBooks))
	m.okuList.Title = fmt.Sprintf("Oku (%d)", len(m.okuBooks))
}

func (m *dashboardModel) refreshSearchResultItems() {
	m.applySearchListDensityLayout()

	idx := m.searchList.Index()
	items := make([]list.Item, 0, len(m.searchBooks))
	for _, r := range m.searchBooks {
		items = append(items, searchResultItem{
			result:  r,
			density: m.density,
		})
	}
	m.searchList.SetItems(items)
	if len(items) == 0 {
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(items) {
		idx = len(items) - 1
	}
	m.searchList.Select(idx)
}

func (m *dashboardModel) applySearchListDensityLayout() {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	if m.density == densityVerbose {
		delegate.SetSpacing(1)
	} else {
		delegate.SetSpacing(0)
	}

	delegate.Styles.NormalTitle = lipgloss.NewStyle().
		Foreground(colorLightGray).
		Padding(0, 0, 0, 2)
	delegate.Styles.NormalDesc = lipgloss.NewStyle().
		Foreground(colorMidGray).
		Padding(0, 0, 0, 2)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(colorGold).
		Foreground(colorGold).
		Bold(true).
		Padding(0, 0, 0, 1)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(colorGold).
		Foreground(colorMidGray).
		Padding(0, 0, 0, 1)
	delegate.Styles.DimmedTitle = lipgloss.NewStyle().
		Foreground(colorDimGray).
		Padding(0, 0, 0, 2)
	delegate.Styles.DimmedDesc = lipgloss.NewStyle().
		Foreground(colorDarkGray).
		Padding(0, 0, 0, 2)

	m.searchList.SetDelegate(delegate)
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
		m.errMsg = "no book selected"
		return m, nil
	}
	m.loading = true
	return m, changeStatusCmd(m.ctx, m.app, b.Book.ID, status)
}

func (m dashboardModel) changeSelectedSearchStatus(status model.Status) (tea.Model, tea.Cmd) {
	r := m.selectedSearchResult()
	if r == nil {
		m.errMsg = "no search result selected"
		return m, nil
	}
	m.loading = true
	return m, addFromSearchCmd(m.ctx, m.app, r.ID, status)
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

		if shouldUseDemoLocalData() {
			stats, sessions = demoLocalData()
		}

		return localDataLoadedMsg{
			readingStats:   stats,
			recentSessions: sessions,
			timerState:     timer,
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

func searchCmd(ctx context.Context, a *app.App, query string, mode model.SearchMode) tea.Cmd {
	return func() tea.Msg {
		results, err := a.SearchBooks(ctx, query, 10, mode)
		return searchLoadedMsg{
			results: results,
			query:   query,
			mode:    mode,
			err:     err,
		}
	}
}

func updateProgressCmd(ctx context.Context, a *app.App, bookID int, rawPage string) tea.Cmd {
	return func() tea.Msg {
		p, err := model.ParsePage(rawPage)
		if err != nil {
			return opDoneMsg{err: err}
		}
		newPage, err := a.UpdateProgress(ctx, bookID, p)
		if err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{
			info:      fmt.Sprintf("Progress updated to page %d", newPage),
			reload:    true,
			markDirty: true,
		}
	}
}

func submitReviewRatingCmd(ctx context.Context, a *app.App, bookID int, rating float64, review string) tea.Cmd {
	return func() tea.Msg {
		if err := a.ReviewBook(ctx, bookID, rating, review); err != nil {
			return opDoneMsg{err: err}
		}
		info := fmt.Sprintf("Updated review and rating (%s)", model.StarString(rating))
		if strings.TrimSpace(review) == "" {
			info = fmt.Sprintf("Updated rating (%s)", model.StarString(rating))
		}
		return opDoneMsg{
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

func quickProgressCmd(ctx context.Context, a *app.App, bookID int, delta int) tea.Cmd {
	return func() tea.Msg {
		if delta == 0 {
			return opDoneMsg{}
		}
		newPage, err := a.UpdateProgress(ctx, bookID, model.PageUpdate{
			Delta:    delta,
			Relative: true,
		})
		if err != nil {
			return opDoneMsg{err: err}
		}
		sign := ""
		if delta > 0 {
			sign = "+"
		}
		return opDoneMsg{
			info:      fmt.Sprintf("Progress %s%d → page %d", sign, delta, newPage),
			reload:    true,
			markDirty: true,
		}
	}
}

func changeStatusCmd(ctx context.Context, a *app.App, bookID int, status model.Status) tea.Cmd {
	return func() tea.Msg {
		if err := a.ChangeStatus(ctx, bookID, status); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{
			info:      fmt.Sprintf("Status changed to %s", status.Label()),
			reload:    true,
			markDirty: true,
		}
	}
}

func addFromSearchCmd(ctx context.Context, a *app.App, bookID int, status model.Status) tea.Cmd {
	return func() tea.Msg {
		if err := a.ChangeStatus(ctx, bookID, status); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{
			info:      fmt.Sprintf("Added to %s", status.Label()),
			reload:    true,
			markDirty: true,
		}
	}
}

func syncAllAndReloadCmd(ctx context.Context, a *app.App) tea.Cmd {
	return func() tea.Msg {
		if err := a.SyncAll(ctx); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{
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
