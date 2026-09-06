package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TestPaceAndETA pins the estimate the detail pane prints. The pace assumes
// the read started at page 0 (see paceAndETA), so these are the numbers that
// assumption gives.
func TestPaceAndETA(t *testing.T) {
	now := time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local)
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }

	cases := []struct {
		name        string
		started     time.Time
		page, pages int
		want        string
	}{
		{"started today has no pace yet", now.Add(-3 * time.Hour), 40, 300, "started today"},
		{"a day in", day(1), 30, 300, "30 pages/day · ~9 days left (≈ 14 Sep)"},
		{"a week in, rounded to weeks", day(7), 70, 300, "10 pages/day · ~3 weeks left (≈ 28 Sep)"},
		{"months out", day(24), 70, 305, "3 pages/day · ~3 months left (≈ 25 Nov)"},
		{"a page a day is singular", day(20), 20, 40, "1 page/day · ~3 weeks left (≈ 25 Sep)"},
		{"under a page a day, still estimable", day(30), 20, 300, "<1 page/day · ~14 months left (≈ 30 Oct 2027)"},
		{"finished books get the pace only", day(10), 300, 300, "30 pages/day"},
		{"too slow to estimate", day(400), 20, 3000, "<1 page/day · pace too low to estimate"},
		{"past two years is not an estimate", day(2), 2, 3000, "1 page/day"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := paceAndETA(c.started, now, c.page, c.pages); got != c.want {
				t.Fatalf("paceAndETA = %q, want %q", got, c.want)
			}
		})
	}
}

// TestPaceLineOnlyForAnOpenRead checks the row is left out rather than
// guessed at when the data behind it is not there.
func TestPaceLineOnlyForAnOpenRead(t *testing.T) {
	now := time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local)
	started := now.AddDate(0, 0, -10)
	reading := model.UserBook{
		StatusID:      model.StatusCurrentlyReading,
		Book:          model.Book{Pages: 300},
		UserBookReads: []model.UserBookRead{{ProgressPages: 100, StartedAt: &started}},
	}
	if got := paceLine(reading, 100, now); got == "" {
		t.Fatal("an open read with a start date and pages should have a pace")
	}

	noStart := reading
	noStart.UserBookReads = []model.UserBookRead{{ProgressPages: 100}}
	if got := paceLine(noStart, 100, now); got != "" {
		t.Fatalf("no start date should mean no pace, got %q", got)
	}

	noPages := reading
	noPages.Book.Pages = 0
	if got := paceLine(noPages, 100, now); got != "" {
		t.Fatalf("no page count should mean no pace, got %q", got)
	}

	shelved := reading
	shelved.StatusID = model.StatusWantToRead
	if got := paceLine(shelved, 100, now); got != "" {
		t.Fatalf("a book that is not being read has no pace, got %q", got)
	}

	if got := paceLine(reading, 0, now); got != "" {
		t.Fatalf("page 0 has no pace to report, got %q", got)
	}
}

// TestDetailGenresFromCachedTags reads the genres out of the Hardcover
// cached_tags blob, and only the genres.
func TestDetailGenresFromCachedTags(t *testing.T) {
	m := newTestModel()
	book := model.UserBook{
		StatusID: model.StatusRead,
		Book: model.Book{
			ID: 1, Title: "Dune", Pages: 412,
			CachedTags: `{"Genre":[{"tag":"Science Fiction"},{"tag":"Classics"},{"tag":"Fantasy"},
				{"tag":"Adventure"},{"tag":"Space Opera"},{"tag":"Epic"}],
				"Mood":[{"tag":"adventurous"}]}`,
		},
	}
	out := stripANSI(renderUserBook(book, nil, time.Now(), 90, DensityDefault, m.st))

	if !strings.Contains(out, "Science Fiction · Classics · Fantasy · Adventure · Space Opera") {
		t.Fatalf("genres row is missing the first five tags:\n%s", out)
	}
	if strings.Contains(out, "Epic") {
		t.Fatalf("only five genres should be listed:\n%s", out)
	}
	if strings.Contains(out, "adventurous") {
		t.Fatalf("moods are not genres:\n%s", out)
	}

	// No blob, no row.
	book.Book.CachedTags = ""
	if out := stripANSI(renderUserBook(book, nil, time.Now(), 90, DensityDefault, m.st)); strings.Contains(out, "Genres") {
		t.Fatalf("a book with no tags should have no genres row:\n%s", out)
	}
	// A corrupt blob is not a crash and not a row either.
	book.Book.CachedTags = "{not json"
	if out := stripANSI(renderUserBook(book, nil, time.Now(), 90, DensityDefault, m.st)); strings.Contains(out, "Genres") {
		t.Fatalf("a corrupt tags blob should have no genres row:\n%s", out)
	}
}

// TestDetailOmitsRowsItHasNoDataFor checks the pane is short rather than
// full of dashes when a book is barely known.
func TestDetailOmitsRowsItHasNoDataFor(t *testing.T) {
	m := newTestModel()
	bare := model.UserBook{StatusID: model.StatusWantToRead, Book: model.Book{ID: 9, Title: "Ubik"}}
	out := stripANSI(renderUserBook(bare, nil, time.Now(), 90, DensityDefault, m.st))

	for _, unwanted := range []string{"Your rating", "Community", "Released", "Genres", "Your review", "Sessions"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("a book with no data should not have a %q row:\n%s", unwanted, out)
		}
	}
	if !strings.Contains(out, "Unknown author") {
		t.Fatalf("an author is named even when there is none:\n%s", out)
	}
	// Series keeps the grid's shape, so it is the one row drawn empty.
	if !strings.Contains(out, "Series") {
		t.Fatalf("the series row keeps the grid in shape:\n%s", out)
	}
}

// TestDetailGridKeepsAGapBetweenItsColumns checks a left value long enough
// to be cut does not run into the right column's label.
func TestDetailGridKeepsAGapBetweenItsColumns(t *testing.T) {
	m := newTestModel()
	for _, w := range []int{56, 60, 68, 80, 116} {
		rows := gridRows(w, m.st,
			[2]string{"Series", strings.Repeat("A very long series name ", 6)},
			[2]string{"Released", "1848"},
		)
		if len(rows) != 1 {
			t.Fatalf("width %d: got %d rows, want the pair on one", w, len(rows))
		}
		row := stripANSI(rows[0])
		if i := strings.Index(row, "Released"); i < 1 || row[i-1] != ' ' {
			t.Fatalf("width %d: the cut left value runs into the right label: %q", w, row)
		}
		if got := lipgloss.Width(rows[0]); got > w {
			t.Fatalf("width %d: the grid row is %d wide", w, got)
		}
	}
}

// TestDetailShowsOnlyThisBooksSessions filters the shared history down to
// the book on screen.
func TestDetailShowsOnlyThisBooksSessions(t *testing.T) {
	m := newTestModel()
	now := time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local)
	end := now.Add(-30 * time.Minute)
	sessions := []model.ReadingSession{
		{ID: 1, BookID: 1, StartedAt: now.Add(-90 * time.Minute), EndedAt: &end, BookTitle: "Dune"},
		{ID: 2, BookID: 2, StartedAt: now.Add(-48 * time.Hour), EndedAt: &end, BookTitle: "Ubik"},
	}
	book := model.UserBook{StatusID: model.StatusRead, Book: model.Book{ID: 1, Title: "Dune"}}
	out := stripANSI(renderUserBook(book, sessions, now, 90, DensityDefault, m.st))

	if !strings.Contains(out, "Sessions (this book)") || !strings.Contains(out, "Today") {
		t.Fatalf("the book's own session should be listed:\n%s", out)
	}
	if strings.Count(out, "Today") != 1 {
		t.Fatalf("another book's session should not be listed:\n%s", out)
	}
}

// TestSearchDetailNamesTheShelf checks that a result already in the library
// says so, with what the reader made of it.
func TestSearchDetailNamesTheShelf(t *testing.T) {
	m := newTestModel()
	r := model.SearchResult{ID: 7, Title: "Dune", Authors: []string{"Frank Herbert"}, Pages: 412, Slug: "dune", Rating: 4.31, Ratings: 1240}

	out := stripANSI(renderSearchResult(r, nil, 68, m.st))
	if strings.Contains(out, "On your shelf") {
		t.Fatalf("a result that is not in the library should say nothing about a shelf:\n%s", out)
	}
	if !strings.Contains(out, "★ 4.31 (1.2K ratings)") || !strings.Contains(out, "hardcover.app/books/dune") {
		t.Fatalf("the community rating and the link should be there:\n%s", out)
	}

	shelf := map[int]model.UserBook{7: {StatusID: model.StatusRead, Rating: 4.5, Book: model.Book{ID: 7}}}
	if out := stripANSI(renderSearchResult(r, shelf, 68, m.st)); !strings.Contains(out, "On your shelf: Read") || !strings.Contains(out, "4.5") {
		t.Fatalf("a shelved result should name its shelf and rating:\n%s", out)
	}

	// A result with no rating says nothing rather than "n/a".
	unrated := r
	unrated.Rating, unrated.Ratings = 0, 0
	if out := stripANSI(renderSearchResult(unrated, nil, 68, m.st)); strings.Contains(out, "Community") {
		t.Fatalf("an unrated result should have no community row:\n%s", out)
	}
}

// TestDetailRebuildsOnlyWhenSomethingChanged pins the memo: the same
// selection at the same size is rendered once.
func TestDetailRebuildsOnlyWhenSomethingChanged(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setTab(tabReading)

	sel := m.section().Selected()
	first := m.detail.View(sel, tabReading)
	before := m.detail.key
	if m.detail.View(sel, tabReading) != first || m.detail.key != before {
		t.Fatal("rendering the same selection again should not rebuild it")
	}

	// Moving the cursor does.
	send(t, m, runeKey('j'))
	if m.detail.View(m.section().Selected(), tabReading) == first {
		t.Fatal("a new selection should be a new detail")
	}

	// So does a data change, even when the selection has not moved: the row
	// below is rendered from the sessions, which the load has just replaced.
	sel = m.section().Selected()
	m.detail.View(sel, tabReading)
	if strings.Contains(stripANSI(m.detail.View(sel, tabReading)), "Sessions (this book)") {
		t.Fatal("the fixture should start with no sessions for this book")
	}

	end := fixedNow.Add(-30 * time.Minute)
	m.shared.sessions = []model.ReadingSession{
		{ID: 1, BookID: sel.Book.Book.ID, StartedAt: fixedNow.Add(-90 * time.Minute), EndedAt: &end},
	}
	m.broadcast(dataChangedMsg{dataLocal})
	if !strings.Contains(stripANSI(m.detail.View(sel, tabReading)), "Sessions (this book)") {
		t.Fatal("a data change should rebuild the pane, not only mark it")
	}
}

// TestDetailKeepsItsScrollAcrossAReload pins what a background reconcile or
// a finished timer must not do: throw the reader back to the top of a long
// review they were halfway through. A new selection still starts at the top.
func TestDetailKeepsItsScrollAcrossAReload(t *testing.T) {
	m := renderedDashboard(120, 40)
	books := m.shared.reading
	books[0].Review = strings.Repeat("A review long enough to scroll the detail pane. ", 60)
	setLibrary(m, books, m.shared.oku)
	m.setTab(tabReading)

	send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	// The viewport has nothing to scroll until it has been rendered once,
	// which in the program every key press is followed by.
	m.frame()
	for i := 0; i < 5; i++ {
		send(t, m, runeKey('j'))
	}
	m.frame()
	if m.detail.vp.YOffset != 5 {
		t.Fatalf("YOffset = %d after five j, want 5", m.detail.vp.YOffset)
	}

	// A reload of the same book: the sessions under the review change, the
	// reader's place does not.
	end := fixedNow.Add(-30 * time.Minute)
	m.shared.sessions = []model.ReadingSession{
		{ID: 1, BookID: books[0].Book.ID, StartedAt: fixedNow.Add(-90 * time.Minute), EndedAt: &end},
	}
	m.broadcast(dataChangedMsg{dataLocal})
	m.frame()
	if got := m.detail.vp.YOffset; got != 5 {
		t.Fatalf("YOffset = %d after a reload, want the reader left where they were (5)", got)
	}

	// A different book is a different page, and starts at the top.
	send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	send(t, m, runeKey('j'))
	m.frame()
	if got := m.detail.vp.YOffset; got != 0 {
		t.Fatalf("YOffset = %d for a new selection, want 0", got)
	}

	// A rebuild that leaves less content than the offset clamps rather than
	// scrolling past the end.
	send(t, m, runeKey('k'))
	send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	for i := 0; i < 5; i++ {
		send(t, m, runeKey('j'))
	}
	m.frame()
	books[0].Review = "Short."
	setLibrary(m, books, m.shared.oku)
	m.frame()
	if got := m.detail.vp.YOffset; got != 0 {
		t.Fatalf("YOffset = %d once the content got shorter, want it clamped to 0", got)
	}
}

// TestSearchResultOpensInTheDetailPane is the keyboard path the README
// promises: Enter over a result opens it, Esc goes back, and shelving it is
// its own key so Enter is free to mean what it means everywhere else.
func TestSearchResultOpensInTheDetailPane(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}} {
		m := renderedDashboard(size[0], size[1])
		m.setTab(tabSearch)
		s := searchOf(m)
		s.results = []model.SearchResult{{ID: 1, Title: "Dune", Authors: []string{"Frank Herbert"}, Pages: 412, Slug: "dune"}}
		s.rebuildResults()
		s.sub = searchSubResults

		send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.focus != focusDetail {
			t.Fatalf("%dx%d: Enter over a result should open the detail pane, focus = %v", size[0], size[1], m.focus)
		}
		if m.isLoading() {
			t.Fatalf("%dx%d: Enter must not shelve the result", size[0], size[1])
		}
		if !strings.Contains(stripANSI(m.frame()), "hardcover.app/books/dune") {
			t.Fatalf("%dx%d: the result's detail should be on screen:\n%s", size[0], size[1], stripANSI(m.frame()))
		}

		// a shelves it, from either focus.
		if cmd := send(t, m, runeKey('a')); cmd == nil {
			t.Fatalf("%dx%d: a should add the result as reading", size[0], size[1])
		}

		send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
		if m.focus != focusContent {
			t.Fatalf("%dx%d: Esc should go back to the results", size[0], size[1])
		}
	}
}

// TestSlashLeavesTheDetailPane checks that jumping to the search input never
// leaves the keyboard pointed at a pane that no key reaches.
func TestSlashLeavesTheDetailPane(t *testing.T) {
	m := renderedDashboard(120, 40)
	m.setTab(tabReading)
	send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.focus != focusDetail {
		t.Fatal("Enter should focus the detail pane")
	}

	send(t, m, runeKey('/'))
	if m.tab != tabSearch {
		t.Fatalf("/ should open the Search tab, got %v", m.tab)
	}
	if m.focus != focusContent {
		t.Fatalf("/ should give the keyboard back to the content pane, focus = %v", m.focus)
	}
	if !searchOf(m).CapturesKeys() {
		t.Fatal("/ should put the cursor in the search input")
	}

	// The Oku tab reaches the input the same way.
	m = renderedDashboard(120, 40)
	m.setTab(tabOku)
	send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	send(t, m, runeKey('/'))
	if m.tab != tabSearch || m.focus != focusContent || !searchOf(m).CapturesKeys() {
		t.Fatalf("/ from the Oku detail: tab=%v focus=%v capturing=%v", m.tab, m.focus, searchOf(m).CapturesKeys())
	}
}

// TestDetailEmptyStates checks the pane says what to press rather than
// sitting blank.
func TestDetailEmptyStates(t *testing.T) {
	m := renderedDashboard(120, 40)
	setLibrary(m, nil, nil)
	m.setTab(tabReading)
	if out := stripANSI(m.frame()); !strings.Contains(out, "j/k to pick a book") {
		t.Fatalf("an empty shelf's detail pane should say what to press:\n%s", out)
	}

	m.setTab(tabSearch)
	m.shared.recentSearches = []string{"dune", "le guin", "kafka"}
	out := stripANSI(m.frame())
	if !strings.Contains(out, "Type a query") {
		t.Fatalf("the search detail pane should ask for a query:\n%s", out)
	}
	if !strings.Contains(out, "Recent: dune · le guin · kafka") {
		t.Fatalf("the search detail pane should list the recent searches:\n%s", out)
	}
}

// TestTabKeysNeverReachTheSearchInput pins the seam between the tab strip's
// numbers and the search input: 1-5 jump to a tab wherever the keyboard is
// not owned by a text field, and are typed where it is.
func TestTabKeysNeverReachTheSearchInput(t *testing.T) {
	states := map[string]func(*searchSection){
		"normal mode": func(s *searchSection) { s.sub, s.mode = searchSubInput, searchModeNormal },
		"results":     func(s *searchSection) { s.sub = searchSubResults },
	}
	want := map[rune]tab{'1': tabReading, '2': tabOku, '3': tabSearch, '4': tabStats, '5': tabTimer}

	for name, arrange := range states {
		for key, tb := range want {
			m := renderedDashboard(120, 40)
			m.setTab(tabSearch)
			s := searchOf(m)
			s.results = []model.SearchResult{{ID: 1, Title: "Dune"}}
			s.rebuildResults()
			s.input.SetValue("dune")
			arrange(s)

			send(t, m, runeKey(key))
			if m.tab != tb {
				t.Fatalf("%s: %q went to tab %v, want %v", name, key, m.tab, tb)
			}
			if got := searchOf(m).input.Value(); got != "dune" {
				t.Fatalf("%s: %q was typed into the search input, which now reads %q", name, key, got)
			}
		}
	}

	// In insert mode the input owns the keyboard, and a digit is a digit.
	m := renderedDashboard(120, 40)
	m.setTab(tabSearch)
	searchOf(m).focusInput()
	send(t, m, runeKey('1'))
	if m.tab != tabSearch {
		t.Fatalf("a digit typed into the input should not switch tabs, tab = %v", m.tab)
	}
	if got := searchOf(m).input.Value(); got != "1" {
		t.Fatalf("insert mode should type the digit, input = %q", got)
	}
}

// TestTabKeysAndFocusRouting walks the keys the header strip advertises.
func TestTabKeysAndFocusRouting(t *testing.T) {
	m := renderedDashboard(120, 40)

	send(t, m, runeKey('l'))
	if m.tab != tabOku {
		t.Fatalf("l = %v, want Oku", m.tab)
	}
	send(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.tab != tabSearch {
		t.Fatalf("tab = %v, want Search", m.tab)
	}
	send(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.tab != tabOku {
		t.Fatalf("shift+tab = %v, want Oku", m.tab)
	}
	send(t, m, runeKey('h'))
	if m.tab != tabReading {
		t.Fatalf("h = %v, want Reading", m.tab)
	}
	send(t, m, runeKey('5'))
	if m.tab != tabTimer {
		t.Fatalf("5 = %v, want Timer", m.tab)
	}
	// The strip wraps at both ends.
	send(t, m, runeKey('l'))
	if m.tab != tabReading {
		t.Fatalf("l past the last tab = %v, want Reading", m.tab)
	}
	send(t, m, runeKey('h'))
	if m.tab != tabTimer {
		t.Fatalf("h before the first tab = %v, want Timer", m.tab)
	}

	// A tab with no detail cannot be focused into one.
	send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.focus != focusContent {
		t.Fatalf("Enter in the Timer tab should not move the focus, got %v", m.focus)
	}

	// Switching tabs gives the keyboard back to the list.
	send(t, m, runeKey('1'))
	send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.focus != focusDetail {
		t.Fatal("Enter in Reading should focus the detail pane")
	}
	send(t, m, runeKey('2'))
	if m.focus != focusContent {
		t.Fatalf("a new tab should start on its content pane, focus = %v", m.focus)
	}
}

// TestNarrowScreenEnterSwapsThePane checks §3's narrow-terminal rule: below
// the split width, Enter shows the detail in place of the list and Esc puts
// the list back — and the shelf keys work in both.
func TestNarrowScreenEnterSwapsThePane(t *testing.T) {
	m := renderedDashboard(80, 24)
	m.setTab(tabReading)
	if m.lay.Split {
		t.Fatal("80 columns is too narrow to split")
	}

	send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.lay.DetailOnly {
		t.Fatal("Enter on a narrow terminal should give the detail pane the width")
	}
	frame := stripANSI(m.frame())
	if !strings.Contains(frame, "Dune") || strings.Contains(frame, "Foundation") {
		t.Fatalf("the detail pane should have replaced the list:\n%s", frame)
	}

	// The library keys still act on the selection behind it.
	if cmd := send(t, m, runeKey('+')); cmd == nil {
		t.Fatal("+ should still update the selected book's progress")
	}

	send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.lay.DetailOnly || m.focus != focusContent {
		t.Fatal("Esc should put the list back")
	}
	if !strings.Contains(stripANSI(m.frame()), "Foundation") {
		t.Fatal("the list should be back on screen")
	}
}
