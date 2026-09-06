package tui

import (
	"strings"
	"testing"

	"github.com/Kameleon21/oku/internal/model"
	tea "github.com/charmbracelet/bubbletea"
)

// searchModel is a loaded dashboard whose Search tab holds results, on the
// tab named by from.
func searchModel(t *testing.T, from tab, results bool) *Model {
	t.Helper()
	m := renderedDashboard(120, 40)
	if results {
		s := searchOf(m)
		s.results = []model.SearchResult{
			{ID: 1, Title: "Dune"}, {ID: 2, Title: "Dune Messiah"},
		}
		s.rebuildResults()
		s.lastQuery, s.lastMode = "dune", model.SearchModeBook
		s.input.SetValue("dune")
	}
	m.setTab(from)
	return m
}

// TestSearchStateMachine walks every transition of the Search tab's two
// states. There is no NORMAL and no INSERT: the input has the keyboard and
// every key is a character, or the results have it and every key is a
// command.
func TestSearchStateMachine(t *testing.T) {
	tests := []struct {
		name string
		// arrange leaves the dashboard in the state the key is pressed from.
		arrange func(t *testing.T) *Model
		key     tea.KeyMsg
		// want is checked after the key.
		want func(t *testing.T, m *Model)
	}{
		{
			name:    "/ from another tab opens the input with the query kept",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabReading, true) },
			key:     runeKey('/'),
			want: func(t *testing.T, m *Model) {
				wantState(t, m, tabSearch, inputFocused)
				if got := searchOf(m).input.Value(); got != "dune" {
					t.Fatalf("query = %q, want the one already typed", got)
				}
				if got := searchOf(m).input.Position(); got != len("dune") {
					t.Fatalf("cursor at %d, want the end of the query (%d)", got, len("dune"))
				}
			},
		},
		{
			name:    "3 with results lands on them",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabReading, true) },
			key:     runeKey('3'),
			want:    func(t *testing.T, m *Model) { wantState(t, m, tabSearch, resultsFocused) },
		},
		{
			name:    "3 with nothing to read lands in the input",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabReading, false) },
			key:     runeKey('3'),
			want:    func(t *testing.T, m *Model) { wantState(t, m, tabSearch, inputFocused) },
		},
		{
			name:    "l onto the tab lands on the results, never in the input",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabOku, false) },
			key:     runeKey('l'),
			want:    func(t *testing.T, m *Model) { wantState(t, m, tabSearch, resultsFocused) },
		},
		{
			name:    "tab onto the tab lands on the results",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabOku, true) },
			key:     tea.KeyMsg{Type: tea.KeyTab},
			want:    func(t *testing.T, m *Model) { wantState(t, m, tabSearch, resultsFocused) },
		},
		{
			name:    "Esc in the input goes back to the tab it came from",
			arrange: func(t *testing.T) *Model { return inInput(t, searchModel(t, tabOku, true)) },
			key:     tea.KeyMsg{Type: tea.KeyEsc},
			want: func(t *testing.T, m *Model) {
				if m.tab != tabOku {
					t.Fatalf("tab = %v, want the one the input was reached from", m.tab)
				}
				if got := searchOf(m).input.Value(); got != "dune" {
					t.Fatalf("query = %q, want it left in the input", got)
				}
			},
		},
		{
			name:    "↓ in the input with results moves to them",
			arrange: func(t *testing.T) *Model { return inInput(t, searchModel(t, tabReading, true)) },
			key:     tea.KeyMsg{Type: tea.KeyDown},
			want:    func(t *testing.T, m *Model) { wantState(t, m, tabSearch, resultsFocused) },
		},
		{
			name:    "↓ in the input with no results stays put",
			arrange: func(t *testing.T) *Model { return inInput(t, searchModel(t, tabReading, false)) },
			key:     tea.KeyMsg{Type: tea.KeyDown},
			want:    func(t *testing.T, m *Model) { wantState(t, m, tabSearch, inputFocused) },
		},
		{
			name:    "j in the input is a letter",
			arrange: func(t *testing.T) *Model { return inInput(t, searchModel(t, tabReading, true)) },
			key:     runeKey('j'),
			want: func(t *testing.T, m *Model) {
				wantState(t, m, tabSearch, inputFocused)
				if got := searchOf(m).input.Value(); got != "dunej" {
					t.Fatalf("query = %q, want the letter typed", got)
				}
			},
		},
		{
			name:    "? in the input is a character, not the help modal",
			arrange: func(t *testing.T) *Model { return inInput(t, searchModel(t, tabReading, false)) },
			key:     runeKey('?'),
			want: func(t *testing.T, m *Model) {
				if m.topModal() != nil {
					t.Fatalf("top modal = %T, want ? typed", m.topModal())
				}
				if got := searchOf(m).input.Value(); got != "?" {
					t.Fatalf("query = %q, want ?", got)
				}
			},
		},
		{
			name:    "1 in the input is a character, not a tab",
			arrange: func(t *testing.T) *Model { return inInput(t, searchModel(t, tabReading, false)) },
			key:     runeKey('1'),
			want: func(t *testing.T, m *Model) {
				wantState(t, m, tabSearch, inputFocused)
				if got := searchOf(m).input.Value(); got != "1" {
					t.Fatalf("query = %q, want the digit typed", got)
				}
			},
		},
		{
			name:    "ctrl+t cycles the mode while typing",
			arrange: func(t *testing.T) *Model { return inInput(t, searchModel(t, tabReading, false)) },
			key:     tea.KeyMsg{Type: tea.KeyCtrlT},
			want: func(t *testing.T, m *Model) {
				wantState(t, m, tabSearch, inputFocused)
				if got := searchOf(m).queryMode; got != model.SearchModeAuthor {
					t.Fatalf("mode = %q, want %q", got, model.SearchModeAuthor)
				}
			},
		},
		{
			name:    "m while typing is a letter",
			arrange: func(t *testing.T) *Model { return inInput(t, searchModel(t, tabReading, false)) },
			key:     runeKey('m'),
			want: func(t *testing.T, m *Model) {
				if got := searchOf(m).queryMode; got != model.SearchModeBook {
					t.Fatalf("mode = %q, want it unchanged", got)
				}
				if got := searchOf(m).input.Value(); got != "m" {
					t.Fatalf("query = %q, want the letter typed", got)
				}
			},
		},
		{
			name: "↵ in the input searches",
			arrange: func(t *testing.T) *Model {
				m := inInput(t, searchModel(t, tabReading, false))
				searchOf(m).input.SetValue("foundation")
				return m
			},
			key: tea.KeyMsg{Type: tea.KeyEnter},
			want: func(t *testing.T, m *Model) {
				if !searchOf(m).loading {
					t.Fatal("Enter should have started a search")
				}
			},
		},
		{
			name:    "Esc over the results goes back to the input",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabSearch, true) },
			key:     tea.KeyMsg{Type: tea.KeyEsc},
			want:    func(t *testing.T, m *Model) { wantState(t, m, tabSearch, inputFocused) },
		},
		{
			name:    "i over the results goes back to the input",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabSearch, true) },
			key:     runeKey('i'),
			want: func(t *testing.T, m *Model) {
				wantState(t, m, tabSearch, inputFocused)
				if got := searchOf(m).input.Position(); got != len("dune") {
					t.Fatalf("cursor at %d, want the end of the query", got)
				}
			},
		},
		{
			name:    "h over the results walks the strip",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabSearch, true) },
			key:     runeKey('h'),
			want:    func(t *testing.T, m *Model) { wantState(t, m, tabOku, resultsFocused) },
		},
		{
			name:    "m over the results cycles the mode",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabSearch, true) },
			key:     runeKey('m'),
			want: func(t *testing.T, m *Model) {
				wantState(t, m, tabSearch, resultsFocused)
				if got := searchOf(m).queryMode; got != model.SearchModeAuthor {
					t.Fatalf("mode = %q, want %q", got, model.SearchModeAuthor)
				}
			},
		},
		{
			name:    "j over the results moves the cursor",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabSearch, true) },
			key:     runeKey('j'),
			want: func(t *testing.T, m *Model) {
				wantState(t, m, tabSearch, resultsFocused)
				if got := searchOf(m).list.Index(); got != 1 {
					t.Fatalf("cursor at %d, want the second result", got)
				}
			},
		},
		{
			name:    "a over the results shelves one",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabSearch, true) },
			key:     runeKey('a'),
			want: func(t *testing.T, m *Model) {
				if !m.isLoading() {
					t.Fatal("a should have started an add")
				}
			},
		},
		{
			name:    "/ over the results goes back to the input",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabSearch, true) },
			key:     runeKey('/'),
			want:    func(t *testing.T, m *Model) { wantState(t, m, tabSearch, inputFocused) },
		},
		{
			name:    "s over the results syncs",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabSearch, true) },
			key:     runeKey('s'),
			want: func(t *testing.T, m *Model) {
				if !m.syncing {
					t.Fatal("s should start a sync, as it does in every other tab")
				}
			},
		},
		{
			name:    "i from a focused detail pane brings the keyboard back with it",
			arrange: func(t *testing.T) *Model { return inDetail(t, searchModel(t, tabSearch, true)) },
			key:     runeKey('i'),
			want: func(t *testing.T, m *Model) {
				wantState(t, m, tabSearch, inputFocused)
				if m.focus != focusContent {
					t.Fatalf("focus = %v, want the content pane: the input has the keyboard", m.focus)
				}
			},
		},
		{
			name:    "Esc from a focused detail pane goes back to the results",
			arrange: func(t *testing.T) *Model { return inDetail(t, searchModel(t, tabSearch, true)) },
			key:     tea.KeyMsg{Type: tea.KeyEsc},
			want: func(t *testing.T, m *Model) {
				wantState(t, m, tabSearch, resultsFocused)
				if m.focus != focusContent {
					t.Fatalf("focus = %v, want the results", m.focus)
				}
			},
		},
		{
			name:    "↵ over the results opens the detail pane",
			arrange: func(t *testing.T) *Model { return searchModel(t, tabSearch, true) },
			key:     tea.KeyMsg{Type: tea.KeyEnter},
			want: func(t *testing.T, m *Model) {
				if m.focus != focusDetail {
					t.Fatalf("focus = %v, want the detail pane", m.focus)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.arrange(t)
			send(t, m, tt.key)
			tt.want(t, m)
		})
	}
}

// inDetail presses Enter over the results, which moves the keyboard to the
// detail pane.
func inDetail(t *testing.T, m *Model) *Model {
	t.Helper()
	send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.focus != focusDetail {
		t.Fatal("Enter over a result should focus the detail pane")
	}
	return m
}

// inInput presses / to put the cursor in the search input, from wherever the
// model is.
func inInput(t *testing.T, m *Model) *Model {
	t.Helper()
	send(t, m, runeKey('/'))
	if searchOf(m).focus != inputFocused {
		t.Fatal("/ should put the cursor in the search input")
	}
	return m
}

func wantState(t *testing.T, m *Model, tb tab, f searchFocus) {
	t.Helper()
	if m.tab != tb {
		t.Fatalf("tab = %v, want %v", m.tab, tb)
	}
	if got := searchOf(m).focus; got != f {
		t.Fatalf("search focus = %v, want %v", got, f)
	}
	if capturing := searchOf(m).CapturesKeys(); capturing != (f == inputFocused && m.tab == tabSearch) {
		t.Fatalf("CapturesKeys = %v in state %v", capturing, f)
	}
}

// TestSearchInputNeverLeavesTheFocusOnTheDetailPane: the pane that has the
// keyboard is the pane that carries the focus. Typing into an input while
// the detail pane held the focus emptied the selection under it — and below
// 100 columns the detail pane has the whole width, so the results being
// typed into were not on screen at all.
func TestSearchInputNeverLeavesTheFocusOnTheDetailPane(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}} {
		for _, key := range []tea.KeyMsg{runeKey('i'), runeKey('/')} {
			m := searchModel(t, tabSearch, true)
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			inDetail(t, m)
			send(t, m, key)

			if searchOf(m).focus != inputFocused {
				t.Fatalf("%dx%d %q: search focus = %v, want the input", size[0], size[1], key, searchOf(m).focus)
			}
			if m.focus != focusContent {
				t.Fatalf("%dx%d %q: focus = %v, want the pane the input is in", size[0], size[1], key, m.focus)
			}
			if m.lay.DetailOnly {
				t.Fatalf("%dx%d %q: the detail pane still has the width the input is drawn in", size[0], size[1], key)
			}
			// And Esc out of the input is the plain one: back to the tab
			// it was reached from, with the focus still on a pane that has
			// something in it.
			send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
			if m.tab != tabReading || m.focus != focusContent {
				t.Fatalf("%dx%d %q: Esc left tab=%v focus=%v, want the tab it came from", size[0], size[1], key, m.tab, m.focus)
			}
		}
	}
}

// TestSearchEscTwiceLeavesTheTab is the way out from the middle of a query:
// once back to the input, once off the tab.
func TestSearchEscTwiceLeavesTheTab(t *testing.T) {
	m := searchModel(t, tabReading, true)
	send(t, m, runeKey('3'))
	wantState(t, m, tabSearch, resultsFocused)

	send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	wantState(t, m, tabSearch, inputFocused)

	send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.tab != tabReading {
		t.Fatalf("tab after the second Esc = %v, want the one the tab was reached from", m.tab)
	}
}

// TestSearchResultsLandOnlyOnAWaitingReader pins where a result puts the
// keyboard: on the results when the reader is still waiting in the input on
// this tab, and nowhere else.
func TestSearchResultsLandOnlyOnAWaitingReader(t *testing.T) {
	loaded := searchLoadedMsg{
		results: []model.SearchResult{{ID: 1, Title: "Dune"}},
		query:   "dune",
		mode:    model.SearchModeBook,
	}

	// Waiting in the input: the results take the keyboard.
	m := inInput(t, searchModel(t, tabReading, false))
	send(t, m, loaded)
	wantState(t, m, tabSearch, resultsFocused)

	// Gone to another tab before it landed: the results are applied, and the
	// reader is left where they went.
	m = inInput(t, searchModel(t, tabReading, false))
	send(t, m, runeKey('1')) // typed into the query, not a tab
	m.setTab(tabReading)     // the reader moves on
	send(t, m, loaded)
	if m.tab != tabReading {
		t.Fatalf("tab = %v, want the reader left where they went", m.tab)
	}
	if len(searchOf(m).results) != 1 {
		t.Fatalf("results = %d, want the search applied behind their back", len(searchOf(m).results))
	}
}

// TestSearchErrorKeepsTheResultsOnScreen: a failed search leaves what is
// already there, says so in the status bar, and does not move the keyboard.
func TestSearchErrorKeepsTheResultsOnScreen(t *testing.T) {
	m := searchModel(t, tabSearch, true)
	send(t, m, runeKey('i'))
	m.shared.loaded = true
	searchOf(m).loading = true
	m.inflight = 1

	send(t, m, searchLoadedMsg{
		query: "dune",
		mode:  model.SearchModeBook,
		seq:   searchOf(m).seq,
		err:   errString("network down"),
	})

	wantState(t, m, tabSearch, inputFocused)
	if len(searchOf(m).results) != 2 {
		t.Fatalf("results = %d, want the two already on screen", len(searchOf(m).results))
	}
	if m.toast.level != toastError || !strings.Contains(m.toast.text, "network down") {
		t.Fatalf("toast = %+v, want the failure reported", m.toast)
	}
}

// errString is an error with a fixed message.
type errString string

func (e errString) Error() string { return string(e) }
