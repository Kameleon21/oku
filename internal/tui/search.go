package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
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

// ── Search section ─────────────────────────────────────────────────────────

// searchSection is the search card and its results. The input has a normal
// and an insert mode, vim style; the results are a list of their own.
type searchSection struct {
	sh *shared
	st styles

	input textinput.Model
	list  list.Model

	results   []model.SearchResult
	sub       searchSubFocus
	mode      searchInputMode
	queryMode model.SearchMode
	// focused reports whether the section has the focus: results landing
	// while it does take the cursor, results landing elsewhere do not.
	focused bool

	loading      bool
	loadingQuery string
	// seq stamps each search; anything but the latest result is dropped.
	seq int

	lastQuery string
	lastMode  model.SearchMode
}

func newSearchSection(sh *shared, st styles) *searchSection {
	l := newList(st)
	// The search results panel has no card label, so it prints this title as
	// its own one-line header.
	l.Title = model.SearchModeBook.Label() + " Results"

	in := textinput.New()
	in.Placeholder = "Search books..."
	in.CharLimit = 120
	in.Prompt = "/ "
	in.PromptStyle = st.inputPrompt
	in.TextStyle = st.inputText
	// Suggestions are the user's own search history, loaded with the rest of
	// the local data; there are none until they have searched for something.
	in.ShowSuggestions = true

	return &searchSection{
		sh:        sh,
		st:        st,
		input:     in,
		list:      l,
		queryMode: model.SearchModeBook,
	}
}

func (s *searchSection) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return s.handleKey(msg)
	case searchLoadedMsg:
		return s.applyLoaded(msg)
	case dataChangedMsg:
		var cmd tea.Cmd
		if msg.kind == dataDensity {
			cmd = s.rebuildResults()
		}
		s.updateSuggestions()
		return cmd
	case list.FilterMatchesMsg:
		// Carries no list id: only the list that is filtering asked for it.
		if s.list.FilterState() == list.Unfiltered {
			return nil
		}
	}
	var cmd tea.Cmd
	if s.sub == searchSubResults {
		s.list, cmd = s.list.Update(msg)
	} else {
		s.input, cmd = s.input.Update(msg)
	}
	return cmd
}

func (s *searchSection) handleKey(msg tea.KeyMsg) tea.Cmd {
	k := keysFor(s)
	if s.sub == searchSubInput {
		if s.mode == searchModeNormal {
			switch {
			case key.Matches(msg, k.SearchInsert):
				s.enterInsertMode()
			case key.Matches(msg, k.SearchAppend):
				s.enterInsertMode()
				s.input.CursorEnd()
			case key.Matches(msg, k.SearchMode):
				return s.setQueryMode(s.queryMode.Next())
			case key.Matches(msg, k.SearchModeBook):
				return s.setQueryMode(model.SearchModeBook)
			case key.Matches(msg, k.SearchModeAuthor):
				return s.setQueryMode(model.SearchModeAuthor)
			case key.Matches(msg, k.SearchModeGenre):
				return s.setQueryMode(model.SearchModeGenre)
			case key.Matches(msg, k.SearchSubmit):
				return s.submit()
			case key.Matches(msg, k.Back, k.Up):
				return request(reqSwitchTab{step: -1})
			case key.Matches(msg, k.Down):
				if s.hasResults() {
					s.sub = searchSubResults
					return nil
				}
				return request(reqSwitchTab{step: +1})
			}
			return nil
		}

		// Insert mode.
		switch {
		case key.Matches(msg, k.SearchSubmit):
			return s.submit()
		case key.Matches(msg, k.Back):
			s.enterNormalMode()
			return nil
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return cmd
	}

	// searchSubResults
	switch {
	case key.Matches(msg, k.SearchBack):
		s.sub = searchSubInput
		s.enterNormalMode()
		return nil
	case key.Matches(msg, k.AddReading):
		if r := s.selected(); r != nil {
			return request(reqAddFromSearch{result: *r, to: model.StatusCurrentlyReading})
		}
	case key.Matches(msg, k.SetReading):
		return s.addSelected(model.StatusCurrentlyReading)
	case key.Matches(msg, k.SetWant):
		return s.addSelected(model.StatusWantToRead)
	case key.Matches(msg, k.SetFinished):
		return s.addSelected(model.StatusRead)
	case key.Matches(msg, k.SetDNF):
		return s.addSelected(model.StatusDidNotFinish)
	}

	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return cmd
}

// inputRow is the search card's body: the modes and the input on one line.
func (s *searchSection) inputRow() string {
	mode := s.st.dim.Render("[NORMAL]")
	if s.mode == searchModeInsert {
		mode = s.st.keyHint.Render("[INSERT]")
	}
	queryMode := s.st.keyHint.Render("[" + s.queryMode.Label() + "]")
	return "  " + mode + " " + queryMode + " " + s.input.View()
}

// View is the results panel: the search in progress, the empty state, or
// the titled list.
func (s *searchSection) View(int, int) string {
	if s.loading {
		query := s.loadingQuery
		if strings.TrimSpace(query) == "" {
			query = s.lastQuery
		}
		if strings.TrimSpace(query) == "" {
			query = "..."
		}
		return "\n  " + s.sh.spin.View() + " " + strings.ToLower(s.queryMode.Label()) +
			" search (" + s.queryMode.Description() + ") for " + fmt.Sprintf("%q", query)
	}

	if len(s.list.Items()) == 0 {
		if strings.TrimSpace(s.lastQuery) == "" {
			return s.st.dim.Render(
				fmt.Sprintf("  %s mode (%s). Type a query and press Enter.",
					strings.ToLower(s.queryMode.Label()), s.queryMode.Description(),
				),
			)
		}
		return s.st.dim.Render(fmt.Sprintf("  No results for %q", s.lastQuery))
	}

	return s.st.listHeader.Render(s.list.Title) + "\n" + s.list.View()
}

// Resize sizes the results list; resizeInput the input on the card.
func (s *searchSection) Resize(w, h int) { s.list.SetSize(w, h) }

func (s *searchSection) resizeInput(w int) { s.input.Width = w }

func (s *searchSection) Keys(k *keyMap) {
	sectionHint := hint("section", k.PrevSection, k.NextSection)
	switch {
	case s.sub == searchSubResults:
		k.Up.SetHelp("k", "navigate")
		k.Down.SetHelp("j", "navigate")
		// h and left go back to the input here, so the previous-section
		// binding is left with the one key it really has.
		k.PrevSection.SetKeys("shift+tab")
		k.PrevSection.SetHelp("S-Tab", "previous section")
		k.SearchBack.SetHelp("Esc/h", "back to input")
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
			hintAs("Esc", "back", k.SearchBack),
		}
	case s.mode == searchModeInsert:
		// ? is typed here, not a shortcut.
		k.Back.SetHelp("Esc", "normal")
		enable(&k.SearchSubmit, &k.Back)
		k.short = []key.Binding{k.SearchSubmit, k.Back}
	default:
		// j goes down into the results (or on to the next section), k
		// up to the previous one: one label that fits both.
		k.Up.SetHelp("k", "results / section")
		k.Down.SetHelp("j", "results / section")
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
}

func (s *searchSection) Focus() { s.focused = true }

// Blur leaves the input: a section switch takes the cursor with it.
func (s *searchSection) Blur() {
	s.focused = false
	s.input.Blur()
}

func (s *searchSection) CapturesKeys() bool {
	return s.sub == searchSubInput && s.mode == searchModeInsert
}

func (s *searchSection) Title() string { return "Search" }

func (s *searchSection) Selected() selection { return selection{Result: s.selected()} }

// inResults reports whether j/k act on the results.
func (s *searchSection) inResults() bool { return s.sub == searchSubResults }

// focusInput jumps into the input in insert mode, keeping whatever query was
// already typed.
func (s *searchSection) focusInput() {
	s.sub = searchSubInput
	s.enterInsertMode()
	s.input.CursorEnd()
	s.updateSuggestions()
}

// ── Search helpers ─────────────────────────────────────────────────────────

func (s *searchSection) hasResults() bool {
	return len(s.list.Items()) > 0
}

// submit asks for a search. An in-flight search is not a reason to refuse:
// seq drops whichever result is superseded, so a typo can be corrected
// immediately.
func (s *searchSection) submit() tea.Cmd {
	query := strings.TrimSpace(s.input.Value())
	if query == "" {
		return request(reqToast{toastError, "search query cannot be empty"})
	}

	// Reuse in-memory results for the same query instead of refetching.
	if strings.EqualFold(query, strings.TrimSpace(s.lastQuery)) &&
		s.queryMode == s.lastMode && len(s.list.Items()) > 0 {
		s.sub = searchSubResults
		s.mode = searchModeNormal
		s.input.Blur()
		return request(reqToast{toastInfo, fmt.Sprintf("%s mode: showing cached results for %q",
			strings.ToLower(s.queryMode.Label()),
			query,
		)})
	}

	s.loading = true
	s.loadingQuery = query
	s.refreshTitle()
	s.seq++
	return request(reqSearch{query: query, mode: s.queryMode, seq: s.seq})
}

// applyLoaded takes a search result: the latest one replaces the list and
// takes the cursor if the section has the focus, an older one is dropped.
func (s *searchSection) applyLoaded(msg searchLoadedMsg) tea.Cmd {
	if msg.seq != s.seq {
		// A newer search superseded this one.
		return nil
	}
	s.loading = false
	s.loadingQuery = ""
	if msg.err != nil {
		// The previous results are still on screen, so the header keeps
		// naming them - including how many there are.
		s.refreshTitle()
		return request(reqToast{toastError, msg.err.Error()})
	}
	s.lastQuery = msg.query
	s.lastMode = msg.mode
	s.results = msg.results
	cmd := s.rebuildResults()
	s.refreshTitle()
	if len(msg.results) > 0 && s.focused {
		// Only take focus if the user is still in the search section.
		s.sub = searchSubResults
		s.enterNormalMode()
	}
	return tea.Batch(cmd, request(reqSearchDone{query: msg.query, mode: msg.mode, results: len(msg.results)}))
}

// setQueryMode switches the mode the next search runs in and asks for the
// toast that says so.
func (s *searchSection) setQueryMode(mode model.SearchMode) tea.Cmd {
	if mode == "" {
		mode = model.SearchModeBook
	}
	// Picking the mode that is already set still says so: the key did
	// something, and the toast is the only place the mode is spelled out.
	s.queryMode = mode
	// The results on screen were fetched in the old mode, so the header is
	// left naming them; only the next search renames it.
	s.refreshTitle()
	s.input.Placeholder = searchPlaceholderForMode(mode)
	s.updateSuggestions()
	return request(reqToast{toastInfo, fmt.Sprintf("Search mode: %s (%s)", strings.ToLower(mode.Label()), mode.Description())})
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

// updateSuggestions feeds the input its completions: the search history,
// plus the library's authors in author mode.
func (s *searchSection) updateSuggestions() {
	seen := map[string]struct{}{}
	out := make([]string, 0, 12)

	push := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}

	for _, q := range s.sh.recentSearches {
		push(q)
	}
	if s.queryMode == model.SearchModeAuthor {
		for _, b := range append(append([]model.UserBook(nil), s.sh.reading...), s.sh.oku...) {
			for _, a := range b.Book.Authors {
				push(a)
			}
		}
	}

	if len(out) > 12 {
		out = out[:12]
	}
	s.input.SetSuggestions(out)
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

// refreshTitle names the results the panel is actually showing: the mode
// they were fetched with and how many came back, not the mode the user has
// switched to since.
func (s *searchSection) refreshTitle() {
	if s.loading {
		s.list.Title = fmt.Sprintf("%s Results (loading...)", s.queryMode.Label())
		return
	}
	mode := s.lastMode
	if mode == "" {
		mode = s.queryMode
	}
	s.list.Title = fmt.Sprintf("%s Results (%d)", mode.Label(), len(s.results))
}

func (s *searchSection) enterNormalMode() {
	s.mode = searchModeNormal
	s.input.Blur()
}

func (s *searchSection) enterInsertMode() {
	s.mode = searchModeInsert
	s.input.Focus()
}

// rebuildResults refreshes the rows from results at the current density,
// keeping the cursor. The returned command must be run: with filtering
// enabled, SetItems reapplies an active filter.
func (s *searchSection) rebuildResults() tea.Cmd {
	spacing := 0
	if s.sh.density == DensityVerbose {
		spacing = 1
	}
	s.list.SetDelegate(newListDelegate(spacing, s.st))

	idx := s.list.Index()
	items := make([]list.Item, 0, len(s.results))
	for _, r := range s.results {
		items = append(items, searchResultItem{result: r, density: s.sh.density})
	}
	cmd := s.list.SetItems(items)
	if len(items) == 0 {
		return cmd
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(items) {
		idx = len(items) - 1
	}
	s.list.Select(idx)
	return cmd
}

func (s *searchSection) selected() *model.SearchResult {
	item := s.list.SelectedItem()
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

// addSelected asks to shelve the selected result.
func (s *searchSection) addSelected(status model.Status) tea.Cmd {
	r := s.selected()
	if r == nil {
		return request(reqToast{toastError, "no search result selected"})
	}
	return request(reqAddFromSearch{result: *r, to: status})
}
