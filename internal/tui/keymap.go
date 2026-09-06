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
	TabJump                  key.Binding // 1-5: straight to a tab
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
	SearchInsert, SearchAppend key.Binding
	SearchMode                 key.Binding
	SearchSubmit, AddReading   key.Binding
	SearchBack                 key.Binding // results back to the input

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
		PrevSection:  bind("h", "previous tab", "h", "left", "shift+tab"),
		NextSection:  bind("l", "next tab", "l", "right", "tab"),
		TabJump:      bind("1-5", "jump to a tab", "1", "2", "3", "4", "5"),
		Search:       bind("/", "search", "/"),
		ScrollTop:    bind("g", "top", "g", "home"),
		ScrollBottom: bind("G", "bottom", "G", "end"),
		HalfPageUp:   bind("C-u", "half page up", "ctrl+u", "pgup"),
		HalfPageDown: bind("C-d", "half page down", "ctrl+d", "pgdown"),

		Details:      bind("↵", "detail", "enter"),
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

		SearchInsert: bind("i", "insert", "i"),
		SearchAppend: bind("a", "append", "a"),
		SearchMode:   bind("m", "cycle mode", "m"),
		SearchSubmit: bind("↵", "search", "enter"),
		// Enter opens a result in the detail pane, the way it opens a book
		// everywhere else, so shelving one has a key of its own.
		AddReading: bind("a", "add as reading", "a"),
		SearchBack: bind("Esc", "back to input", "esc", "h", "left"),

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
			k.TabJump,
			hint("tab", k.PrevSection, k.NextSection),
			hintAs("Tab/S-Tab", "tab (alias)", k.NextSection, k.PrevSection),
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
// disabled key neither matches in a handler nor shows in the help. The top
// modal answers when one is up; the active section otherwise.
func (m *Model) activeKeys() keyMap {
	k := newKeyMap()
	enable(&k.ForceQuit)
	if top := m.topModal(); top != nil {
		top.Keys(&k)
		return k
	}
	m.sectionKeys(&k)
	return k
}

// sectionKeys is the keymap with no modal up: the section's own keys, undo
// while a toast offers one, and the detail pane's scroll keys over the top
// of them while it has the focus. Undo lasts as long as that toast, and
// never takes a letter away from the search input.
func (m *Model) sectionKeys(k *keyMap) {
	if m.undo != nil && !m.section().CapturesKeys() {
		enable(&k.Undo)
	}
	m.section().Keys(k)
	if m.focus == focusDetail {
		m.detail.Keys(k)
	}
}

// keysBehind is the keymap the help modal describes: what the focus under
// it understands.
func (m *Model) keysBehind() keyMap {
	k := newKeyMap()
	enable(&k.ForceQuit)
	if n := len(m.modals); n > 1 {
		m.modals[n-2].Keys(&k)
		return k
	}
	m.sectionKeys(&k)
	return k
}

// helpBarWidth is the room the footer hints have. Zero means unbounded, which
// is what a model that has not seen a window size yet should use.
func (m *Model) helpBarWidth() int {
	if m.lay.W <= 0 {
		return 0
	}
	return max(minHelpBarWidth, m.lay.W-2)
}

// renderHelpBar lays out as many hints as fit in limit columns and marks the
// rest with an ellipsis. help.Model can do this itself, but when the ellipsis
// is the thing that does not fit its width check falls through and appends the
// hint anyway, leaving a dangling separator to be cut mid-word; the bar drops
// whole hints here instead, so the width is always honoured and the cut is
// always marked.
func (m *Model) renderHelpBar(bindings []key.Binding, limit int) string {
	h := m.help
	h.Width = 0 // The loop below owns the width.

	view := h.ShortHelpView(bindings)
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
func (m *Model) helpBindings() []key.Binding {
	return m.activeKeys().ShortHelp()
}
