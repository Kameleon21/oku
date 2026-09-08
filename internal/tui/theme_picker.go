package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/config"
	"github.com/charmbracelet/x/ansi"
)

func currentThemeSetting() string {
	if name := ActiveThemeName(); name != "" {
		return name
	}
	if dark, pinned := PinnedDark(); pinned {
		if dark {
			return "dark"
		}
		return "light"
	}
	return "auto"
}

// Preview uses the same rebuild path for every section, cached page and widget.
// Terminal detection is remembered separately so cancelling back to auto never
// inherits the background of the last previewed named palette.
func (m *Model) previewTheme(name string) tea.Cmd {
	if err := ApplyThemeSetting(name); err != nil {
		return nil
	}
	m.isDark, m.themePinned = PinnedDark()
	if !m.themePinned {
		m.isDark = m.terminalDark
	}
	m.st = newStyles(ActiveTheme(m.isDark))
	m.help = newHelp(m.st)
	m.help.SetWidth(m.helpBarWidth())
	m.shared.spin.Style = m.st.spinner
	cmd := m.broadcast(stylesChangedMsg{st: m.st})
	if !m.themePinned {
		return tea.Batch(cmd, tea.RequestBackgroundColor)
	}
	return cmd
}

func (m *Model) openThemePicker() tea.Cmd {
	p := &themePicker{original: currentThemeSetting(), preview: m.previewTheme, save: config.SetTheme}
	p.input = textinput.New()
	p.input.Prompt = "Search: "
	p.input.Placeholder = "type a theme name"
	p.input.CharLimit = 64
	p.input.SetVirtualCursor(false)
	p.input.SetStyles(m.st.textInputStyles(m.st.modalKey, m.st.modalValue, m.st.modalDim))
	for i, name := range ThemeSettings() {
		if name == p.original {
			p.cursor = i
		}
	}
	return m.push(p)
}

type themePicker struct {
	original string
	input    textinput.Model
	cursor   int
	err      string
	preview  func(string) tea.Cmd
	save     func(string) error
}

func themeMatches(query, name string) bool {
	remaining := []rune(strings.ToLower(strings.TrimSpace(query)))
	for _, r := range strings.ToLower(name) {
		if len(remaining) > 0 && remaining[0] == r {
			remaining = remaining[1:]
		}
	}
	return len(remaining) == 0
}
func (p *themePicker) matches() []string {
	var out []string
	for _, name := range ThemeSettings() {
		if themeMatches(p.input.Value(), name) {
			out = append(out, name)
		}
	}
	return out
}
func (p *themePicker) Init() tea.Cmd { return p.input.Focus() }
func (p *themePicker) Cursor() *tea.Cursor {
	cur := p.input.Cursor()
	if cur != nil {
		cur.X += modalContentX
		cur.Y += modalContentY + 1
	}
	return cur
}
func themePickerWidth(lay layout) int {
	if lay.W <= 0 {
		return 62
	}
	return max(10, min(62, lay.W-2))
}
func (p *themePicker) Resize(lay layout) {
	p.input.SetWidth(max(1, modalInnerW(themePickerWidth(lay))-8))
}
func (p *themePicker) Keys(k *keyMap) {
	enable(&k.Up, &k.Down, &k.Select, &k.Back)
	k.Up.SetKeys("up", "ctrl+p", "shift+tab")
	k.Down.SetKeys("down", "ctrl+n", "tab")
	k.Up.SetHelp("↑", "move")
	k.Down.SetHelp("↓", "move")
	k.Select.SetHelp("Enter", "apply")
	k.Back.SetHelp("Esc", "cancel")
	k.short = []key.Binding{hint("move", k.Up, k.Down), k.Select, k.Back}
}
func (p *themePicker) Update(msg tea.Msg) (bool, tea.Cmd) {
	if change, ok := msg.(stylesChangedMsg); ok {
		p.input.SetStyles(change.st.textInputStyles(change.st.modalKey, change.st.modalValue, change.st.modalDim))
		return false, nil
	}
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		matches := p.matches()
		k := keysFor(p)
		switch {
		case key.Matches(msg, k.Back):
			return true, p.preview(p.original)
		case key.Matches(msg, k.Select):
			if len(matches) == 0 {
				return false, nil
			}
			if err := p.save(matches[p.cursor]); err != nil {
				p.err = "Could not save: " + err.Error()
				return false, nil
			}
			return true, p.preview(matches[p.cursor])
		case key.Matches(msg, k.Down):
			if len(matches) > 0 {
				p.cursor = (p.cursor + 1) % len(matches)
			}
		case key.Matches(msg, k.Up):
			if len(matches) > 0 {
				p.cursor = (p.cursor + len(matches) - 1) % len(matches)
			}
		default:
			before := p.input.Value()
			var cmd tea.Cmd
			p.input, cmd = p.input.Update(msg)
			if before == p.input.Value() {
				return false, cmd
			}
			p.cursor = 0
			p.err = ""
			matches = p.matches()
			if len(matches) > 0 {
				return false, tea.Batch(cmd, p.preview(matches[0]))
			}
			return false, cmd
		}
		if len(matches) > 0 {
			return false, p.preview(matches[p.cursor])
		}
		return false, nil
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return false, cmd
}
func (p *themePicker) View(lay layout, st styles) string {
	inner := modalInnerW(themePickerWidth(lay))
	cut := func(s string) string { return ansi.Truncate(s, inner, "…") }
	rows := []string{st.modalDim.Render(cut("Applied: " + p.original)), p.input.View()}
	matches := p.matches()
	slots := len(ThemeSettings())
	if lay.H > 0 {
		slots = max(1, min(slots, lay.H-12))
	}
	start := max(0, p.cursor-slots+1)
	for i := start; i < min(len(matches), start+slots); i++ {
		name := matches[i]
		row := "  " + name
		if name == p.original {
			row += " (current)"
		}
		if i == p.cursor {
			row = "> " + strings.TrimPrefix(row, "  ")
			rows = append(rows, st.modalKey.Render(cut(row)))
		} else {
			rows = append(rows, st.modalValue.Render(cut(row)))
		}
	}
	if len(matches) == 0 {
		rows = append(rows, st.modalDim.Render(cut("No matching themes")))
	}
	status := fmt.Sprintf("%d matches · live preview", len(matches))
	if p.err != "" {
		rows = append(rows, st.modalError.Render(cut(p.err)))
	} else {
		rows = append(rows, st.modalDim.Render(cut(status)))
	}
	rows = append(rows, st.modalDim.Render(cut("↑/↓ move · Enter apply · Esc cancel")))
	return renderModalPanel(cut("Choose theme"), strings.Join(rows, "\n"), themePickerWidth(lay), st)
}
