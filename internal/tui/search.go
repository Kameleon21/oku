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
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// recentSearchesKey is the store state key the search history is kept under,
// and maxRecentSearches caps how much of it is remembered.
const (
	recentSearchesKey = "recent_searches"
	maxRecentSearches = 10
)

// searchFocus is where the keyboard is on the Search tab. There are two
// states and no modes: either the input has it and every key is a
// character, or the results have it and every key is a command.
//
// resultsFocused is the zero value, and the state the tab is entered in:
// walking the strip with h/l must never leave a text input holding the
// keyboard, since the input would swallow the very keys that walk back
// off it.
type searchFocus int

const (
	resultsFocused searchFocus = iota
	inputFocused
)

// searchModes is the segmented control's order, which is also the order
// SearchMode.Next() cycles in.
var searchModes = []model.SearchMode{model.SearchModeBook, model.SearchModeAuthor, model.SearchModeGenre}

// segmentLabel is a mode's name in the segmented control. SearchModeBook is
// BOOK to the CLI, which searches books; on screen it is the title field the
// query is matched against, next to Author and Genre.
func segmentLabel(mode model.SearchMode) string {
	switch mode {
	case model.SearchModeAuthor:
		return "Author"
	case model.SearchModeGenre:
		return "Genre"
	default:
		return "Title"
	}
}

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

// searchSection is the query input, the mode control under it and the
// results. The keyboard is in one of two places (see searchFocus).
type searchSection struct {
	sh *shared
	st styles

	input textinput.Model
	list  list.Model

	results   []model.SearchResult
	focus     searchFocus
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
	// The list's own / filter is unreachable by design — / is the dashboard's
	// search key — so filtering is off: with it on, SetItems answers with
	// filter commands nothing here runs.
	l.SetFilteringEnabled(false)

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
		// Another list's filter: this one has none (see newSearchSection).
		return nil
	}
	var cmd tea.Cmd
	if s.focus == resultsFocused {
		s.list, cmd = s.list.Update(msg)
	} else {
		s.input, cmd = s.input.Update(msg)
	}
	return cmd
}

func (s *searchSection) handleKey(msg tea.KeyMsg) tea.Cmd {
	k := keysFor(s)
	if s.focus == inputFocused {
		switch {
		case key.Matches(msg, k.SearchSubmit):
			return s.submit()
		case key.Matches(msg, k.SearchMode):
			// ctrl+t here: m is a letter, and Tab is the input's own
			// suggestion-accept.
			return s.setQueryMode(s.queryMode.Next())
		case key.Matches(msg, k.Back):
			// The query is left in the input: coming back to the tab finds
			// it where it was.
			return request(reqSwitchTab{back: true})
		case key.Matches(msg, k.Down):
			// The arrow only — Keys() takes j off this binding here, since
			// a letter belongs to the query.
			if s.hasResults() {
				s.focusResults()
				return nil
			}
		}
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return cmd
	}

	switch {
	case key.Matches(msg, k.SearchInput):
		s.focusInput()
		return nil
	case key.Matches(msg, k.SearchMode):
		return s.setQueryMode(s.queryMode.Next())
	case key.Matches(msg, k.AddReading):
		return s.addSelected(model.StatusCurrentlyReading)
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

// searchChromeRows is what the pane spends above the results: the input,
// the mode control, and a blank line between them and the list.
const searchChromeRows = 3

// View is the whole pane: the query, the segmented mode control with the
// state of the search on its right, and the results themselves.
func (s *searchSection) View(w, h int) string {
	return fitBlock(s.input.View()+"\n"+s.controlRow(w)+"\n\n"+s.resultsView(), w, h)
}

// controlRow is the segmented control — "[Title]  Author   Genre" — with
// what the results are doing right-aligned against it.
func (s *searchSection) controlRow(w int) string {
	segments := make([]string, 0, len(searchModes))
	for _, mode := range searchModes {
		label := segmentLabel(mode)
		if mode == s.queryMode {
			// Marked as well as coloured, so the choice survives a terminal
			// without colour.
			segments = append(segments, s.st.tabActive.Render("["+label+"]"))
			continue
		}
		segments = append(segments, s.st.dim.Render(" "+label+" "))
	}

	left := strings.Join(segments, " ")
	right := s.statusText()
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if right == "" || gap < 1 {
		// The control is the row's reason to exist; the status is what goes
		// when they do not both fit.
		return ansi.Truncate(left, max(0, w), "")
	}
	return left + strings.Repeat(" ", gap) + right
}

// statusText is the right of the control row: the search in progress, or
// what the results on screen amount to.
func (s *searchSection) statusText() string {
	switch {
	case s.loading:
		return s.sh.spin.View() + " " + s.st.dim.Render("searching…")
	case len(s.results) > 0:
		return s.st.dim.Render(fmt.Sprintf("%d results", len(s.results)))
	case strings.TrimSpace(s.lastQuery) != "":
		return s.st.dim.Render("no results")
	default:
		return ""
	}
}

// resultsView is the pane below the control row. Results already on screen
// stay there while the next search runs, so the pane does not blank out
// between one query and the next.
func (s *searchSection) resultsView() string {
	if len(s.list.Items()) > 0 {
		return s.list.View()
	}
	if s.loading {
		query := s.loadingQuery
		if strings.TrimSpace(query) == "" {
			query = "…"
		}
		return s.st.dim.Render(fmt.Sprintf("  Searching for %q", query))
	}
	if strings.TrimSpace(s.lastQuery) == "" {
		return s.st.dim.Render("  Type a query and press Enter.")
	}
	return s.st.dim.Render(fmt.Sprintf("  No results for %q", s.lastQuery))
}

// Resize gives the results the rows the input row and the header do not
// take, and the input the columns its badges leave.
func (s *searchSection) Resize(w, h int) tea.Cmd {
	s.list.SetSize(w, max(1, h-searchChromeRows))
	// The prompt takes the front of the row; the input takes what is left
	// instead of being cut off mid-placeholder.
	s.input.Width = max(4, w-6)
	return nil
}

func (s *searchSection) Keys(k *keyMap) {
	if s.focus == inputFocused {
		// The input owns the keyboard here, so every key advertised is one
		// it does not swallow: no letters, no digits.
		k.Down.SetKeys("down")
		k.Down.SetHelp("↓", "results")
		k.SearchMode.SetKeys("ctrl+t")
		k.SearchMode.SetHelp("C-t", "cycle mode")
		k.Back.SetHelp("Esc", "back")
		enable(&k.SearchSubmit, &k.SearchMode, &k.Back)
		if s.hasResults() {
			enable(&k.Down)
		}
		k.short = []key.Binding{k.SearchSubmit, k.SearchMode, k.Down, k.Back}
		return
	}

	// h and l walk the tab strip here as they do everywhere else: Esc and i
	// are the way back to the input. A key that walked left off the strip in
	// every other tab and stopped in a text field in this one would make the
	// walk one-way.
	//
	// The keys that act on a result are only advertised when there is one:
	// an empty results pane is the state the tab is first entered in, and it
	// has nothing to open, shelve or scroll.
	k.SearchMode.SetKeys("m")
	k.SearchMode.SetHelp("m", "cycle mode")
	enable(&k.Help, &k.SearchInput, &k.SearchMode, &k.NextSection, &k.PrevSection,
		&k.TabJump, &k.Density)
	k.short = []key.Binding{
		k.Help,
		hintAs("Esc/i", "input", k.SearchInput),
		k.SearchMode,
		hint("tab", k.PrevSection, k.NextSection),
		k.Density,
	}
	if !s.hasResults() {
		return
	}

	k.Up.SetHelp("k", "navigate")
	k.Down.SetHelp("j", "navigate")
	k.SetReading.SetHelp("g", "add as reading")
	k.SetWant.SetHelp("w", "add as want to read")
	k.SetFinished.SetHelp("f", "add as finished")
	k.SetDNF.SetHelp("d", "add as did not finish")
	enable(&k.Up, &k.Down, &k.Details, &k.AddReading, &k.SetReading, &k.SetWant,
		&k.SetFinished, &k.SetDNF)
	k.short = []key.Binding{
		k.Help,
		hint("navigate", k.Down, k.Up),
		k.Details,
		k.AddReading,
		hint("status", k.SetReading, k.SetWant, k.SetFinished, k.SetDNF),
		k.SearchMode,
		hintAs("Esc/i", "input", k.SearchInput),
		hint("tab", k.PrevSection, k.NextSection),
		k.Density,
	}
}

func (s *searchSection) Focus() { s.focused = true }

// Blur leaves the input: a section switch takes the cursor with it, and the
// tab is left in the state it is entered in, so walking back onto it never
// finds a text field holding the keyboard.
func (s *searchSection) Blur() {
	s.focused = false
	s.focusResults()
}

func (s *searchSection) CapturesKeys() bool { return s.focus == inputFocused }

func (s *searchSection) Title() string { return "Search" }

// Selected is the result the detail pane shows: only once the cursor is in
// the results, so typing a query does not flicker the pane through every
// title the cursor happens to land on.
func (s *searchSection) Selected() selection {
	if s.focus != resultsFocused {
		return selection{}
	}
	return selection{Result: s.selected()}
}

// focusInput puts the cursor at the end of whatever query is already typed.
func (s *searchSection) focusInput() {
	s.focus = inputFocused
	s.input.Focus()
	s.input.CursorEnd()
	s.updateSuggestions()
}

// focusInputIfEmpty is the Search tab reached by its own number: with
// results to read the cursor stays on them, with nothing to read it goes
// where the reader is about to type.
func (s *searchSection) focusInputIfEmpty() {
	if !s.hasResults() {
		s.focusInput()
	}
}

// focusResults hands the keyboard to the list, where every key is a command
// again.
func (s *searchSection) focusResults() {
	s.focus = resultsFocused
	s.input.Blur()
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
		s.focusResults()
		return request(reqToast{toastInfo, fmt.Sprintf("%s mode: showing cached results for %q",
			strings.ToLower(s.queryMode.Label()),
			query,
		)})
	}

	s.loading = true
	s.loadingQuery = query
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
		// The results already on screen stay there, and the control row
		// keeps counting them.
		return request(reqToast{toastError, msg.err.Error()})
	}
	s.lastQuery = msg.query
	s.lastMode = msg.mode
	s.results = msg.results
	cmd := s.rebuildResults()
	if len(msg.results) > 0 && s.focused && s.focus == inputFocused {
		// Only take the keyboard if the reader is still on this tab and
		// still in the input: results landing behind their back must not
		// move it.
		s.focusResults()
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
