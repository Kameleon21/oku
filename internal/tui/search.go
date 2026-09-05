package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// recentSearchesKey is the store state key the search history is kept under,
// and maxRecentSearches caps how much of it is remembered.
const (
	recentSearchesKey = "recent_searches"
	maxRecentSearches = 10
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

type searchResultItem struct {
	result  model.SearchResult
	density Density
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
			rating += fmt.Sprintf(" (%s ratings)", format.Count(i.result.Ratings))
		}
	}

	switch i.density {
	case DensityCompact:
		return author
	case DensityDefault:
		return rating + " · " + author
	case DensityVerbose:
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

// focusSearchInput jumps to the search card in insert mode, keeping whatever
// query was already typed.
func (m *Model) focusSearchInput() {
	m.setSection(sectionSearch)
	m.searchSub = searchSubInput
	m.enterSearchInsertMode()
	m.searchInput.CursorEnd()
	m.updateSearchSuggestions()
}

func (m Model) handleSearchKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
				cmd := m.setSearchQueryMode(m.searchQueryMode.Next())
				return m, cmd
			case key.Matches(msg, k.SearchModeBook):
				cmd := m.setSearchQueryMode(model.SearchModeBook)
				return m, cmd
			case key.Matches(msg, k.SearchModeAuthor):
				cmd := m.setSearchQueryMode(model.SearchModeAuthor)
				return m, cmd
			case key.Matches(msg, k.SearchModeGenre):
				cmd := m.setSearchQueryMode(model.SearchModeGenre)
				return m, cmd
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

func (m Model) searchSectionContent(w int) string {
	mode := dimStyleTUI.Render("[NORMAL]")
	if m.searchMode == searchModeInsert {
		mode = keyStyle.Render("[INSERT]")
	}
	queryMode := keyStyle.Render("[" + m.searchQueryMode.Label() + "]")
	return "  " + mode + " " + queryMode + " " + m.searchInput.View()
}

func (m Model) searchPanelView() string {
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

// ── Search helpers ─────────────────────────────────────────────────────────

func (m Model) hasSearchResults() bool {
	return len(m.searchList.Items()) > 0
}

// submitSearch starts a search. An in-flight search is not a reason to
// refuse: searchSeq drops whichever result is superseded, so a typo can be
// corrected immediately.
func (m *Model) submitSearch() tea.Cmd {
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

// setSearchQueryMode switches the mode the next search runs in and returns
// the toast that says so.
func (m *Model) setSearchQueryMode(mode model.SearchMode) tea.Cmd {
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
func (m *Model) addRecentSearchQuery(query string) tea.Cmd {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	m.recentSearches = dedupeQueries(append([]string{query}, m.recentSearches...))
	return saveRecentSearchesCmd(m.app, m.recentSearches)
}

// mergeRecentSearches keeps this session's queries ahead of the ones read back
// from the store, so a load landing mid-session cannot drop them.
func (m *Model) mergeRecentSearches(loaded []string) {
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

func (m *Model) updateSearchSuggestions() {
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
func (m *Model) refreshSearchTitle() {
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

func (m *Model) enterSearchNormalMode() {
	m.searchMode = searchModeNormal
	m.searchInput.Blur()
}

func (m *Model) enterSearchInsertMode() {
	m.searchMode = searchModeInsert
	m.searchInput.Focus()
}

func (m *Model) refreshSearchResultItems() tea.Cmd {
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

func (m *Model) applySearchListDensityLayout() {
	spacing := 0
	if m.density == DensityVerbose {
		spacing = 1
	}
	m.searchList.SetDelegate(newListDelegate(spacing))
}

func (m Model) selectedSearchResult() *model.SearchResult {
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

func (m Model) changeSelectedSearchStatus(status model.Status) (tea.Model, tea.Cmd) {
	r := m.selectedSearchResult()
	if r == nil {
		cmd := m.showToast(toastError, "no search result selected")
		return m, cmd
	}
	return m.startOp(addFromSearchCmd(m.ctx, m.app, r.ID, status))
}
