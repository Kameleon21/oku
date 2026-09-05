package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// minHelpBarWidth keeps the footer hints readable on a very narrow terminal.
const minHelpBarWidth = 20

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

// hasEnabled reports whether any key in the group applies right now.
func (g helpGroup) hasEnabled() bool {
	for _, b := range g.bindings {
		if b.Enabled() {
			return true
		}
	}
	return false
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
		HalfPageUp:   bind("C-u", "half page up", "ctrl+u", "pgup"),
		HalfPageDown: bind("C-d", "half page down", "ctrl+d", "pgdown"),

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
		ReviewSave:      bind("C-s", "save", "ctrl+s"),
		ReviewNextField: bind("Tab", "next field", "tab"),
		ReviewPrevField: bind("S-Tab", "previous field", "shift+tab"),
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

// upDownDesc labels the merged j/k row. activeKeys gives the two the same
// description wherever they are a pair; anywhere else (the defaults, say)
// the row can only say that they move.
func (k keyMap) upDownDesc() string {
	if up, down := k.Up.Help().Desc, k.Down.Help().Desc; up == down {
		return up
	}
	return "move"
}

// helpGroups is the help modal's structure. Every binding is listed; the
// ones the current focus has not enabled are drawn dimmed.
func (k keyMap) helpGroups() []helpGroup {
	return []helpGroup{
		{"Navigation", []key.Binding{
			hint(k.upDownDesc(), k.Down, k.Up),
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
func (m Model) activeKeys() keyMap {
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
		case m.searchMode == searchModeInsert:
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
			k.Up.SetHelp("k", "section")
			k.Down.SetHelp("j", "section")
			k.Timer.SetHelp("t", "stop timer")
			enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.NextSection, &k.PrevSection, &k.Search,
				&k.Timer, &k.TimerStop)
			k.short = []key.Binding{k.Help, hint("stop", k.Timer, k.TimerStop), sectionHint, k.Search, k.Quit}
		default:
			k.Up.SetHelp("k", "section")
			k.Down.SetHelp("j", "section")
			k.Timer.SetHelp("t", "choose + start")
			enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.NextSection, &k.PrevSection, &k.Search, &k.Timer)
			k.short = []key.Binding{k.Help, k.Timer, sectionHint, k.Search, k.Quit}
		}

	default:
		k.Up.SetHelp("k", "section")
		k.Down.SetHelp("j", "section")
		enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.NextSection, &k.PrevSection, &k.Search)
		k.short = []key.Binding{k.Help, sectionHint, hintAs("Tab", "next", k.NextSection), k.Search, k.Quit}
	}
	return k
}

// helpBarWidth is the room the footer hints have. Zero means unbounded, which
// is what a model that has not seen a window size yet should use.
func (m Model) helpBarWidth() int {
	if m.width <= 0 {
		return 0
	}
	return max(minHelpBarWidth, m.width-2)
}

// contextHelpBar renders the hints for whatever has focus, on one line.
func (m Model) contextHelpBar() string {
	return " " + m.renderHelpBar(m.helpBindings())
}

// renderHelpBar lays out as many hints as fit and marks the rest with an
// ellipsis. help.Model can do this itself, but when the ellipsis is the thing
// that does not fit its width check falls through and appends the hint anyway,
// leaving a dangling separator to be cut mid-word; the bar drops whole hints
// here instead, so the width is always honoured and the cut is always marked.
func (m Model) renderHelpBar(bindings []key.Binding) string {
	h := m.help
	h.Width = 0 // The loop below owns the width.

	view := h.ShortHelpView(bindings)
	limit := m.helpBarWidth()
	if limit <= 0 || lipgloss.Width(view)+2 <= limit {
		return view
	}

	ellipsis := m.st.dim.Render("…")
	for n := len(bindings) - 1; n >= 1; n-- {
		candidate := h.ShortHelpView(bindings[:n]) + " " + ellipsis
		if lipgloss.Width(candidate)+2 <= limit {
			return candidate
		}
	}
	return ellipsis
}

// helpBindings returns the hints for the focused section.
func (m Model) helpBindings() []key.Binding {
	return m.activeKeys().ShortHelp()
}
