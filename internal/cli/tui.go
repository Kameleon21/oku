package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type viewMode int

const (
	modeLibrary viewMode = iota
	modeUpdatePage
)

const (
	backgroundSyncWindow = 10 * time.Minute
	backgroundCheckEvery = 1 * time.Minute
)

type focusPanel int

const (
	focusReading focusPanel = iota
	focusOku
	focusSearchInput
	focusSearchResults
)

type searchInputMode int

const (
	searchModeNormal searchInputMode = iota
	searchModeInsert
)

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
	result model.SearchResult
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
	return fmt.Sprintf("%s | ID: %d", author, i.result.ID)
}

func (i searchResultItem) FilterValue() string {
	return i.result.Title + " " + strings.Join(i.result.Authors, " ")
}

type libraryLoadedMsg struct {
	reading []model.UserBook
	oku     []model.UserBook
	err     error
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

type dashboardModel struct {
	app *app.App

	mode    viewMode
	focus   focusPanel
	loaded  bool
	loading bool

	width  int
	height int

	readingList list.Model
	okuList     list.Model
	searchList  list.Model
	searchInput textinput.Model
	pageInput   textinput.Model
	spin        spinner.Model

	readingBooks []model.UserBook
	okuBooks     []model.UserBook

	pendingBookID  int
	dirty          bool
	lastMutationAt time.Time

	lastQuery string
	infoMsg   string
	errMsg    string

	searchLoading      bool
	searchLoadingQuery string
	searchMode         searchInputMode
	searchQueryMode    model.SearchMode
	recentSearches     []string
	density            outputDensity

	showHelp bool
}

func newDashboardModel(a *app.App) dashboardModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(0)

	// Custom delegate styles — warm library palette
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

	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(colorGold)

	return dashboardModel{
		app:             a,
		mode:            modeLibrary,
		focus:           focusReading,
		searchMode:      searchModeNormal,
		searchQueryMode: model.SearchModeBook,
		density:         currentOutputDensity(),
		readingList:     newList("Reading"),
		okuList:         newList("Oku"),
		searchList:      newList("Search Results"),
		searchInput:     searchIn,
		pageInput:       pageIn,
		spin:            s,
	}
}

func (m dashboardModel) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, loadLibraryCmd(m.app, false), backgroundCheckCmd())
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeUpdatePage:
		return m.updatePageMode(msg)
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
		m.refreshListItems()
		m.updateSearchSuggestions()
		m.errMsg = ""
		return m, nil
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
		items := make([]list.Item, 0, len(msg.results))
		for _, r := range msg.results {
			items = append(items, searchResultItem{result: r})
		}
		m.searchList.SetItems(items)
		m.searchList.Title = fmt.Sprintf("%s Results (%d)", m.searchQueryMode.Label(), len(items))
		if len(items) > 0 {
			m.focus = focusSearchResults
		}
		m.errMsg = ""
		if len(items) == 0 {
			m.infoMsg = fmt.Sprintf("%s mode: no results for %q", strings.ToLower(m.searchQueryMode.Label()), msg.query)
		} else {
			m.infoMsg = fmt.Sprintf("%s mode: loaded %d results", strings.ToLower(m.searchQueryMode.Label()), len(items))
		}
		m.addRecentSearchQuery(msg.query)
		m.updateSearchSuggestions()
		return m, nil
	case backgroundCheckMsg:
		if m.dirty && !m.loading && time.Since(m.lastMutationAt) >= backgroundSyncWindow {
			m.loading = true
			m.dirty = false
			return m, tea.Batch(loadLibraryCmd(m.app, true), backgroundCheckCmd())
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
			return m, loadLibraryCmd(m.app, false)
		}
		return m, nil
	case tea.KeyMsg:
		// When help modal is open, only allow dismiss or quit
		if m.showHelp {
			switch msg.String() {
			case "?", "esc":
				m.showHelp = false
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}

		// ── Focus-specific key handling ──
		if m.focus == focusSearchInput {
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
				case "esc":
					m.focus = focusReading
					m.searchInput.Blur()
					return m, nil
				case "h", "H", "left":
					m.focusPrevPane()
					return m, nil
				case "l", "L", "right":
					m.focusNextPane()
					return m, nil
				}
				return m, nil
			}

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

		if m.focus == focusSearchResults {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "?":
				m.showHelp = true
				return m, nil
			case "esc":
				m.focus = focusSearchInput
				m.enterSearchNormalMode()
				return m, nil
			case "enter":
				if r := m.selectedSearchResult(); r != nil {
					m.loading = true
					return m, addFromSearchCmd(m.app, r.ID, model.StatusCurrentlyReading)
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
			case "h", "H", "left":
				m.focusPrevPane()
				return m, nil
			case "l", "L", "right":
				m.focusNextPane()
				return m, nil
			}

			var cmd tea.Cmd
			m.searchList, cmd = m.searchList.Update(msg)
			return m, cmd
		}

		// ── focusReading / focusOku ──
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = true
			return m, nil
		case "h", "H", "left":
			m.focusPrevPane()
			return m, nil
		case "l", "L", "right":
			m.focusNextPane()
			return m, nil
		case "/":
			m.focus = focusSearchInput
			m.enterSearchInsertMode()
			m.searchInput.SetValue("")
			m.updateSearchSuggestions()
			m.errMsg = ""
			return m, nil
		case "r":
			m.loading = true
			return m, loadLibraryCmd(m.app, true)
		case "s":
			m.loading = true
			return m, syncAllAndReloadCmd(m.app)
		case "z":
			m.cycleDensity()
			return m, nil
		case "enter":
			if m.focus == focusOku {
				return m.changeSelectedLibraryStatus(model.StatusCurrentlyReading)
			}
			return m.changeSelectedLibraryStatus(model.StatusWantToRead)
		case "+", "=":
			if b := m.selectedLibraryBook(); b != nil {
				m.loading = true
				return m, quickProgressCmd(m.app, b.Book.ID, +10)
			}
		case "-":
			if b := m.selectedLibraryBook(); b != nil {
				m.loading = true
				return m, quickProgressCmd(m.app, b.Book.ID, -10)
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
	}

	var cmd tea.Cmd
	switch m.focus {
	case focusReading:
		m.readingList, cmd = m.readingList.Update(msg)
	case focusOku:
		m.okuList, cmd = m.okuList.Update(msg)
	}
	return m, cmd
}

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
	return searchCmd(m.app, query, m.searchQueryMode)
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

func (m *dashboardModel) focusNextPane() {
	switch m.focus {
	case focusReading:
		m.focus = focusOku
		m.searchInput.Blur()
	case focusOku:
		m.focus = focusSearchInput
		m.enterSearchNormalMode()
	case focusSearchInput:
		m.searchInput.Blur()
		if m.hasSearchResults() {
			m.focus = focusSearchResults
		} else {
			m.focus = focusReading
		}
	case focusSearchResults:
		m.focus = focusReading
	}
}

func (m *dashboardModel) focusPrevPane() {
	switch m.focus {
	case focusReading:
		if m.hasSearchResults() {
			m.focus = focusSearchResults
			m.searchInput.Blur()
		} else {
			m.focus = focusSearchInput
			m.enterSearchNormalMode()
		}
	case focusOku:
		m.focus = focusReading
		m.searchInput.Blur()
	case focusSearchInput:
		m.focus = focusOku
		m.searchInput.Blur()
	case focusSearchResults:
		m.focus = focusSearchInput
		m.enterSearchNormalMode()
	}
}

func (m dashboardModel) updatePageMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case backgroundCheckMsg:
		if m.dirty && !m.loading && time.Since(m.lastMutationAt) >= backgroundSyncWindow {
			m.loading = true
			m.dirty = false
			return m, tea.Batch(loadLibraryCmd(m.app, true), backgroundCheckCmd())
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
			return m, loadLibraryCmd(m.app, false)
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
			return m, updateProgressCmd(m.app, m.pendingBookID, raw)
		}
	}

	var cmd tea.Cmd
	m.pageInput, cmd = m.pageInput.Update(msg)
	return m, cmd
}

func (m dashboardModel) View() string {
	if !m.loaded {
		return "\n  " + m.spin.View() + " Loading dashboard..."
	}

	// ── Status bar: app name (left) + status message (right) ──
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

	// ── Body ──
	var body string
	switch m.mode {
	case modeUpdatePage:
		pagePrompt := "\n " + keyStyle.Render("Page update") + " " + m.pageInput.View() +
			dimStyleTUI.Render("  (Enter submit, Esc cancel)")
		body = statusBar + "\n" + m.renderLibrary() + pagePrompt
	default:
		body = statusBar + "\n" + m.renderLibrary() + "\n" + m.libraryHelp()
	}

	if m.showHelp {
		return m.overlayModal(m.renderHelpModal())
	}
	return body
}

func (m dashboardModel) renderLibrary() string {
	totalW := max(60, m.width-2)
	totalH := max(18, m.height-8)
	leftW := max(28, totalW*2/5)

	// 3 left panels: reading + oku split height, search input is compact
	searchH := 1
	listH := max(6, (totalH-searchH-6)/2) // -6 accounts for 3 panel borders (2 each)

	readingView := m.readingList.View()
	okuView := m.okuList.View()

	// Force consistent width and height on all left panels
	readingBox := panelStyle.Width(leftW).Height(listH).Render(readingView)
	okuBox := panelStyle.Width(leftW).Height(listH).Render(okuView)
	searchBox := panelStyle.Width(leftW).Height(searchH).Render(m.searchInputWithModeBadge())
	if m.focus == focusReading {
		readingBox = panelFocusedStyle.Width(leftW).Height(listH).Render(readingView)
	}
	if m.focus == focusOku {
		okuBox = panelFocusedStyle.Width(leftW).Height(listH).Render(okuView)
	}
	if m.focus == focusSearchInput {
		searchBox = panelFocusedStyle.Width(leftW).Height(searchH).Render(m.searchInputWithModeBadge())
	}

	leftCol := lipgloss.JoinVertical(lipgloss.Left, readingBox, okuBox, searchBox)
	leftH := lipgloss.Height(leftCol)

	// Right column: details or search results depending on focus
	rightW := max(28, m.width-lipgloss.Width(leftCol)-2)

	var rightContent string
	switch m.focus {
	case focusSearchInput, focusSearchResults:
		rightContent = m.searchPanelView()
	default:
		rightContent = m.detailsView()
	}

	rightStyle := panelStyle
	if m.focus == focusSearchResults {
		rightStyle = panelFocusedStyle
	}
	rightPanel := rightStyle.
		Width(rightW).
		Height(leftH - 2).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightPanel)
}

func (m dashboardModel) detailsView() string {
	b := m.selectedLibraryBook()
	if b == nil {
		return dimStyleTUI.Render("  No book selected")
	}

	var sb strings.Builder

	// Title + author
	sb.WriteString(headStyle.Render(b.Book.Title))
	sb.WriteString("\n")
	author := fallback(b.Book.AuthorString(), "Unknown author")
	sb.WriteString(dimStyleTUI.Render(author))
	sb.WriteString("\n\n")

	// Labeled fields
	writeField := func(label, value string) {
		sb.WriteString(labelStyle.Render(fmt.Sprintf("  %-10s ", label)))
		sb.WriteString(valueStyle.Render(value))
		sb.WriteString("\n")
	}

	writeField("Status", b.StatusID.Label())

	// Progress with visual bar
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

	sb.WriteString("\n")
	sb.WriteString(dimStyleTUI.Render("  Quick: Enter toggle  +/- page  u custom  g/w/f/d status  x remove  z density"))

	return sb.String()
}

func (m dashboardModel) searchInputWithModeBadge() string {
	mode := dimStyleTUI.Render("[NORMAL]")
	if m.searchMode == searchModeInsert {
		mode = keyStyle.Render("[INSERT]")
	}
	queryMode := keyStyle.Render("[" + m.searchQueryMode.Label() + "]")
	return mode + " " + queryMode + " " + m.searchInput.View()
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
			{"h / l", "Move pane left / right"},
			{"← / →", "Move pane left / right"},
			{"j / k", "Move up / down in list"},
			{"/", "Focus search input"},
			{"Esc", "Back / cancel"},
		}),
		section("Library Actions", [][2]string{
			{"Enter", "Toggle reading / oku"},
			{"+ / -", "Quick page update (+/-10)"},
			{"u", "Update page progress"},
			{"g", "Set status: reading"},
			{"w", "Set status: want to read"},
			{"f", "Set status: finished"},
			{"d", "Set status: did not finish"},
			{"x", "Remove from library"},
			{"z", "Cycle density: compact/default/verbose"},
		}),
		section("Data", [][2]string{
			{"r", "Refresh from cache"},
			{"s", "Full sync with Hardcover"},
		}),
		section("Search Panel", [][2]string{
			{"Enter", "Execute search / add book"},
			{"i / a", "Enter insert mode"},
			{"Esc", "Insert -> normal / back"},
			{"h/l or ←/→", "Pane nav in normal mode"},
			{"m", "Cycle mode: book/author/genre"},
			{"1/2/3", "Set mode: book/author/genre"},
			{"g/w/f/d", "Add result with status"},
			{"Esc (results)", "Back to search input"},
		}),
		section("General", [][2]string{
			{"?", "Toggle this help"},
			{"q", "Quit"},
		}),
	)

	footer := "\n" + dimStyleTUI.Render("Press ? or Esc to close")
	return helpModalStyle.Render(body + footer)
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

func (m dashboardModel) libraryHelp() string {
	switch m.focus {
	case focusSearchInput:
		if m.searchMode == searchModeInsert {
			return renderHelpBar([][2]string{
				{"↵", "search"},
				{"Esc", "normal mode"},
				{"?", "help"},
			})
		}
		return renderHelpBar([][2]string{
			{"↵", "search"},
			{"i/a", "insert"},
			{"m", "mode"},
			{"1/2/3", "book/author/genre"},
			{"h/l", "pane"},
			{"Esc", "back"},
			{"?", "help"},
		})
	case focusSearchResults:
		return renderHelpBar([][2]string{
			{"↵", "add reading"},
			{"g", "reading"},
			{"w", "want"},
			{"f", "finished"},
			{"d", "dnf"},
			{"z", "density"},
			{"h/l", "pane"},
			{"Esc", "back"},
			{"?", "help"},
		})
	default:
		return renderHelpBar([][2]string{
			{"h/l", "pane"},
			{"/", "search"},
			{"↵", "toggle read/oku"},
			{"+/-", "page"},
			{"u", "update"},
			{"g", "reading"},
			{"w", "want"},
			{"f", "finished"},
			{"d", "dnf"},
			{"x", "remove"},
			{"z", "density"},
			{"r", "refresh"},
			{"s", "sync"},
			{"?", "help"},
			{"q", "quit"},
		})
	}
}

func (m *dashboardModel) resize() {
	totalW := max(60, m.width-2)
	totalH := max(18, m.height-8)

	// Left column: ~40% width; right column gets the rest
	leftW := max(28, totalW*2/5)
	rightW := max(28, totalW-leftW-3) // 3 accounts for borders

	// 3 left panels: reading + oku split height, search input is compact
	searchH := 1
	listH := max(6, (totalH-searchH-6)/2) // -6 accounts for 3 panel borders (2 each)

	m.readingList.SetSize(leftW, listH)
	m.okuList.SetSize(leftW, listH)
	m.searchList.SetSize(rightW, 2*listH+searchH+4) // matches right panel content height
}

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

func (m dashboardModel) selectedLibraryBook() *model.UserBook {
	var item list.Item
	if m.focus == focusOku {
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
	return m, changeStatusCmd(m.app, b.Book.ID, status)
}

func (m dashboardModel) changeSelectedSearchStatus(status model.Status) (tea.Model, tea.Cmd) {
	r := m.selectedSearchResult()
	if r == nil {
		m.errMsg = "no search result selected"
		return m, nil
	}
	m.loading = true
	return m, addFromSearchCmd(m.app, r.ID, status)
}

func loadLibraryCmd(a *app.App, refresh bool) tea.Cmd {
	return func() tea.Msg {
		reading, err := a.ListBooks(ctx(), model.StatusCurrentlyReading, refresh)
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		oku, err := a.ListBooks(ctx(), model.StatusWantToRead, refresh)
		if err != nil {
			return libraryLoadedMsg{err: err}
		}
		return libraryLoadedMsg{
			reading: reading,
			oku:     oku,
		}
	}
}

func searchCmd(a *app.App, query string, mode model.SearchMode) tea.Cmd {
	return func() tea.Msg {
		results, err := a.SearchBooks(ctx(), query, 10, mode)
		return searchLoadedMsg{
			results: results,
			query:   query,
			mode:    mode,
			err:     err,
		}
	}
}

func updateProgressCmd(a *app.App, bookID int, rawPage string) tea.Cmd {
	return func() tea.Msg {
		p, err := model.ParsePage(rawPage)
		if err != nil {
			return opDoneMsg{err: err}
		}
		newPage, err := a.UpdateProgress(ctx(), bookID, p)
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

func quickProgressCmd(a *app.App, bookID int, delta int) tea.Cmd {
	return func() tea.Msg {
		if delta == 0 {
			return opDoneMsg{}
		}
		newPage, err := a.UpdateProgress(ctx(), bookID, model.PageUpdate{
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

func changeStatusCmd(a *app.App, bookID int, status model.Status) tea.Cmd {
	return func() tea.Msg {
		if err := a.ChangeStatus(ctx(), bookID, status); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{
			info:      fmt.Sprintf("Status changed to %s", status.Label()),
			reload:    true,
			markDirty: true,
		}
	}
}

func addFromSearchCmd(a *app.App, bookID int, status model.Status) tea.Cmd {
	return func() tea.Msg {
		if err := a.ChangeStatus(ctx(), bookID, status); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{
			info:      fmt.Sprintf("Added to %s", status.Label()),
			reload:    true,
			markDirty: true,
		}
	}
}

func syncAllAndReloadCmd(a *app.App) tea.Cmd {
	return func() tea.Msg {
		if err := a.SyncAll(ctx()); err != nil {
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

func runDashboard() error {
	a, err := initApp()
	if err != nil {
		return err
	}
	defer a.Store.Close()

	p := tea.NewProgram(newDashboardModel(a), tea.WithAltScreen())
	_, err = p.Run()
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
