package cli

import (
	"fmt"
	"strings"

	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type viewMode int

const (
	modeLibrary viewMode = iota
	modeSearch
	modeUpdatePage
)

type focusPanel int

const (
	focusReading focusPanel = iota
	focusOku
	focusSearchInput
	focusSearchResults
)

var (
	panelFocusedStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("212"))
	panelStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("239"))
	headStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	dimStyleTUI       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	infoStyleTUI      = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	errorStyleTUI     = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
)

type userBookItem struct {
	book   model.UserBook
	active bool
}

func (i userBookItem) Title() string {
	marker := "  "
	if i.active {
		marker = "* "
	}
	return marker + i.book.Book.Title
}

func (i userBookItem) Description() string {
	author := i.book.Book.AuthorString()
	if author == "" {
		author = "Unknown author"
	}
	return fmt.Sprintf("%s | %s", author, i.book.Progress())
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

type activeLoadedMsg struct {
	id int
}

type searchLoadedMsg struct {
	results []model.SearchResult
	query   string
	err     error
}

type opDoneMsg struct {
	info     string
	err      error
	reload   bool
	activeID int
}

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

	readingBooks []model.UserBook
	okuBooks     []model.UserBook

	activeID      int
	pendingBookID int

	lastQuery string
	infoMsg   string
	errMsg    string
}

func newDashboardModel(a *app.App) dashboardModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true

	newList := func(title string) list.Model {
		l := list.New(nil, delegate, 40, 12)
		l.Title = title
		l.SetShowStatusBar(false)
		l.SetFilteringEnabled(true)
		l.DisableQuitKeybindings()
		return l
	}

	searchIn := textinput.New()
	searchIn.Placeholder = "Search books..."
	searchIn.CharLimit = 120

	pageIn := textinput.New()
	pageIn.Placeholder = "370 or +10 or -5"
	pageIn.CharLimit = 32

	return dashboardModel{
		app:         a,
		mode:        modeLibrary,
		focus:       focusReading,
		readingList: newList("Reading"),
		okuList:     newList("Oku"),
		searchList:  newList("Search Results"),
		searchInput: searchIn,
		pageInput:   pageIn,
	}
}

func (m dashboardModel) Init() tea.Cmd {
	return tea.Batch(loadActiveCmd(m.app), loadLibraryCmd(m.app, false))
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeUpdatePage:
		return m.updatePageMode(msg)
	case modeSearch:
		return m.updateSearchMode(msg)
	default:
		return m.updateLibraryMode(msg)
	}
}

func (m dashboardModel) updateLibraryMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
		m.errMsg = ""
		return m, nil
	case activeLoadedMsg:
		m.activeID = msg.id
		m.refreshListItems()
		return m, nil
	case opDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.infoMsg = ""
		} else {
			m.errMsg = ""
			m.infoMsg = msg.info
		}
		if msg.activeID > 0 {
			m.activeID = msg.activeID
			m.refreshListItems()
		}
		if msg.reload {
			m.loading = true
			return m, loadLibraryCmd(m.app, true)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l":
			if m.focus == focusReading {
				m.focus = focusOku
			} else {
				m.focus = focusReading
			}
			return m, nil
		case "shift+tab", "left", "h":
			if m.focus == focusOku {
				m.focus = focusReading
			} else {
				m.focus = focusOku
			}
			return m, nil
		case "/":
			m.mode = modeSearch
			m.focus = focusSearchInput
			m.searchInput.Focus()
			m.searchInput.SetValue("")
			m.errMsg = ""
			return m, nil
		case "r":
			m.loading = true
			return m, loadLibraryCmd(m.app, true)
		case "s":
			m.loading = true
			return m, syncAllAndReloadCmd(m.app)
		case "a":
			if b := m.selectedLibraryBook(); b != nil {
				m.loading = true
				return m, setActiveCmd(m.app, b.Book.ID)
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
		}
	}

	var cmd tea.Cmd
	if m.focus == focusReading {
		m.readingList, cmd = m.readingList.Update(msg)
	} else {
		m.okuList, cmd = m.okuList.Update(msg)
	}
	return m, cmd
}

func (m dashboardModel) updateSearchMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case searchLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.infoMsg = ""
			return m, nil
		}
		m.lastQuery = msg.query
		items := make([]list.Item, 0, len(msg.results))
		for _, r := range msg.results {
			items = append(items, searchResultItem{result: r})
		}
		m.searchList.SetItems(items)
		m.searchList.Title = fmt.Sprintf("Search Results (%d)", len(items))
		if len(items) > 0 {
			m.focus = focusSearchResults
		}
		m.errMsg = ""
		m.infoMsg = fmt.Sprintf("Loaded %d results", len(items))
		return m, nil
	case opDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			m.infoMsg = ""
		} else {
			m.errMsg = ""
			m.infoMsg = msg.info
		}
		if msg.activeID > 0 {
			m.activeID = msg.activeID
		}
		if msg.reload {
			// Keep search view, but refresh local library in background.
			return m, loadLibraryCmd(m.app, true)
		}
		return m, nil
	case libraryLoadedMsg:
		// background refresh after actions in search mode
		if msg.err == nil {
			m.readingBooks = msg.reading
			m.okuBooks = msg.oku
			m.refreshListItems()
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.mode = modeLibrary
			m.focus = focusReading
			m.searchInput.Blur()
			return m, nil
		}

		if m.focus == focusSearchInput {
			switch msg.String() {
			case "enter":
				query := strings.TrimSpace(m.searchInput.Value())
				if query == "" {
					m.errMsg = "search query cannot be empty"
					return m, nil
				}
				m.loading = true
				m.errMsg = ""
				return m, searchCmd(m.app, query)
			case "tab", "down":
				if len(m.searchList.Items()) > 0 {
					m.focus = focusSearchResults
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}

		// focusSearchResults
		switch msg.String() {
		case "tab", "up":
			m.focus = focusSearchInput
			return m, nil
		case "enter":
			if r := m.selectedSearchResult(); r != nil {
				m.loading = true
				return m, addFromSearchCmd(m.app, r.ID, model.StatusCurrentlyReading, true)
			}
		case "a":
			if r := m.selectedSearchResult(); r != nil {
				m.loading = true
				return m, setActiveCmd(m.app, r.ID)
			}
		case "g":
			return m.changeSelectedSearchStatus(model.StatusCurrentlyReading, true)
		case "w":
			return m.changeSelectedSearchStatus(model.StatusWantToRead, false)
		case "f":
			return m.changeSelectedSearchStatus(model.StatusRead, false)
		case "d":
			return m.changeSelectedSearchStatus(model.StatusDidNotFinish, false)
		}
	}

	var cmd tea.Cmd
	if m.focus == focusSearchResults {
		m.searchList, cmd = m.searchList.Update(msg)
	} else {
		m.searchInput, cmd = m.searchInput.Update(msg)
	}
	return m, cmd
}

func (m dashboardModel) updatePageMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
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
		}
		if msg.reload {
			m.loading = true
			return m, loadLibraryCmd(m.app, true)
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
		return "Loading dashboard..."
	}

	title := headStyle.Render("Oku Dashboard")
	if m.loading {
		title += "  " + dimStyleTUI.Render("Loading...")
	}

	notice := ""
	if m.errMsg != "" {
		notice = errorStyleTUI.Render(m.errMsg)
	} else if m.infoMsg != "" {
		notice = infoStyleTUI.Render(m.infoMsg)
	} else {
		notice = dimStyleTUI.Render("Rate-limit friendly cache: status lists auto-refresh only when stale or forced.")
	}

	if m.mode == modeSearch {
		return title + "\n" + notice + "\n\n" + m.renderSearch() + "\n" + m.searchHelp()
	}
	if m.mode == modeUpdatePage {
		return title + "\n" + notice + "\n\n" + m.renderLibrary() + "\n" + infoStyleTUI.Render("Page update: ") + m.pageInput.View() + dimStyleTUI.Render("  (Enter submit, Esc cancel)")
	}
	return title + "\n" + notice + "\n\n" + m.renderLibrary() + "\n" + m.libraryHelp()
}

func (m dashboardModel) renderLibrary() string {
	left := m.readingList.View()
	right := m.okuList.View()

	leftBox := panelStyle.Render(left)
	rightBox := panelStyle.Render(right)
	if m.focus == focusReading {
		leftBox = panelFocusedStyle.Render(left)
	}
	if m.focus == focusOku {
		rightBox = panelFocusedStyle.Render(right)
	}

	top := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
	details := panelStyle.Render(m.detailsView())
	return top + "\n" + details
}

func (m dashboardModel) renderSearch() string {
	inputBox := panelStyle.Render(m.searchInput.View())
	if m.focus == focusSearchInput {
		inputBox = panelFocusedStyle.Render(m.searchInput.View())
	}

	results := m.searchList.View()
	resultsBox := panelStyle.Render(results)
	if m.focus == focusSearchResults {
		resultsBox = panelFocusedStyle.Render(results)
	}

	return headStyle.Render("Search") + "\n" + inputBox + "\n" + resultsBox
}

func (m dashboardModel) detailsView() string {
	b := m.selectedLibraryBook()
	if b == nil {
		return "No book selected."
	}
	lines := []string{
		headStyle.Render(b.Book.Title),
		fmt.Sprintf("Author: %s", fallback(b.Book.AuthorString(), "Unknown author")),
		fmt.Sprintf("Progress: %s", b.Progress()),
		fmt.Sprintf("Status: %s", b.StatusID.Label()),
		fmt.Sprintf("Book ID: %d", b.Book.ID),
	}
	if b.Book.Pages > 0 {
		lines = append(lines, fmt.Sprintf("Pages: %d", b.Book.Pages))
	}
	if b.Book.Slug != "" {
		lines = append(lines, fmt.Sprintf("Slug: %s", b.Book.Slug))
	}
	return strings.Join(lines, "\n")
}

func (m dashboardModel) libraryHelp() string {
	return dimStyleTUI.Render("Home keys: Tab switch pane | / search | a active | u update | g/w/f/d status | r refresh | s sync | q quit")
}

func (m dashboardModel) searchHelp() string {
	return dimStyleTUI.Render("Search keys: Enter search/add-reading | Tab switch input/results | a active | g/w/f/d status | Esc back | q quit")
}

func (m *dashboardModel) resize() {
	totalW := max(60, m.width-2)
	totalH := max(18, m.height-8)
	paneW := max(28, totalW/2-1)
	listH := max(8, totalH/2)

	m.readingList.SetSize(paneW, listH)
	m.okuList.SetSize(paneW, listH)
	m.searchList.SetSize(totalW, max(8, totalH-4))
}

func (m *dashboardModel) refreshListItems() {
	toItems := func(books []model.UserBook) []list.Item {
		items := make([]list.Item, 0, len(books))
		for _, b := range books {
			items = append(items, userBookItem{
				book:   b,
				active: b.Book.ID == m.activeID,
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

func (m dashboardModel) changeSelectedSearchStatus(status model.Status, setActive bool) (tea.Model, tea.Cmd) {
	r := m.selectedSearchResult()
	if r == nil {
		m.errMsg = "no search result selected"
		return m, nil
	}
	m.loading = true
	return m, addFromSearchCmd(m.app, r.ID, status, setActive)
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

func loadActiveCmd(a *app.App) tea.Cmd {
	return func() tea.Msg {
		id, err := a.GetActiveBookID()
		if err != nil {
			return activeLoadedMsg{}
		}
		return activeLoadedMsg{id: id}
	}
}

func searchCmd(a *app.App, query string) tea.Cmd {
	return func() tea.Msg {
		results, err := a.SearchBooks(ctx(), query, 10)
		return searchLoadedMsg{
			results: results,
			query:   query,
			err:     err,
		}
	}
}

func setActiveCmd(a *app.App, bookID int) tea.Cmd {
	return func() tea.Msg {
		if err := a.SetActiveBook(bookID); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{
			info:     fmt.Sprintf("Active book set to ID %d", bookID),
			activeID: bookID,
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
			info:   fmt.Sprintf("Progress updated to page %d", newPage),
			reload: true,
		}
	}
}

func changeStatusCmd(a *app.App, bookID int, status model.Status) tea.Cmd {
	return func() tea.Msg {
		if err := a.ChangeStatus(ctx(), bookID, status); err != nil {
			return opDoneMsg{err: err}
		}
		return opDoneMsg{
			info:   fmt.Sprintf("Status changed to %s", status.Label()),
			reload: true,
		}
	}
}

func addFromSearchCmd(a *app.App, bookID int, status model.Status, setActive bool) tea.Cmd {
	return func() tea.Msg {
		if err := a.ChangeStatus(ctx(), bookID, status); err != nil {
			return opDoneMsg{err: err}
		}
		if setActive {
			if err := a.SetActiveBook(bookID); err != nil {
				return opDoneMsg{err: err}
			}
		}
		msg := fmt.Sprintf("Added to %s", status.Label())
		if setActive {
			msg += " and set active"
		}
		return opDoneMsg{
			info:     msg,
			reload:   true,
			activeID: ternaryInt(setActive, bookID, 0),
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

func ternaryInt(cond bool, a, b int) int {
	if cond {
		return a
	}
	return b
}
