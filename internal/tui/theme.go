package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ── Theme ───────────────────────────────────────────────────────────────────

// Theme names the colours the dashboard and the CLI output are drawn with.
// Every colour is resolved for one background: lipgloss v2 has no adaptive
// colour, so NewTheme is handed the answer instead — the background the
// terminal reports (tea.BackgroundColorMsg for the dashboard, a query at
// startup for the CLI), or the `theme` config key when that pins it (see
// ApplyThemeSetting). The palette is warm on a dark terminal and the same
// hues darkened on a light one, so nothing washes out.
type Theme struct {
	Accent        color.Color // focus, key hints, selection
	Heading       color.Color // titles
	Text          color.Color // body text
	TextMuted     color.Color // descriptions, secondary text
	TextDim       color.Color // hints, counts, subtle text
	Border        color.Color // unfocused borders, empty tracks
	BorderFocused color.Color // focused borders
	Surface       color.Color // status bar and modal background
	Success       color.Color // done, progress filled
	Warning       color.Color // wait, retry
	Error         color.Color // failures, destructive
	Heat1         color.Color // activity ramp, lightest
	Heat2         color.Color
	Heat3         color.Color
	Heat4         color.Color // activity ramp, busiest
}

// NewTheme is the built-in palette in 256-colour indices, resolved for a dark
// or a light terminal. The dark side is the gold-and-olive look the dashboard
// has always had; the light side keeps the hues but drops their luminance so
// they read on white.
func NewTheme(isDark bool) Theme {
	ld := lipgloss.LightDark(isDark)
	// c takes the light index first, as lipgloss.LightDark does.
	c := func(light, dark string) color.Color {
		return ld(lipgloss.Color(light), lipgloss.Color(dark))
	}
	return Theme{
		Accent:        c("130", "179"),
		Heading:       c("235", "223"),
		Text:          c("236", "252"),
		TextMuted:     c("242", "245"),
		TextDim:       c("245", "243"),
		Border:        c("250", "238"),
		BorderFocused: c("130", "179"),
		Surface:       c("254", "236"),
		Success:       c("64", "107"),
		Warning:       c("166", "215"),
		Error:         c("160", "167"),
		// A four-step ramp that stays visible at both ends: the dark side
		// climbs from a muted green to a pale one, the light side from a
		// pale green down to a deep one.
		Heat1: c("150", "65"),
		Heat2: c("107", "71"),
		Heat3: c("64", "113"),
		Heat4: c("22", "156"),
	}
}

// DefaultTheme is the palette for a dark terminal, which is what the
// dashboard draws with until the terminal answers the background query and
// what the CLI falls back to when it is not writing to one.
func DefaultTheme() Theme { return NewTheme(true) }

// ── Styles ──────────────────────────────────────────────────────────────────

// styles is every style the dashboard draws with, derived from one Theme. It
// is a value on the Model rather than a set of package vars because the theme
// is about to become a runtime answer: lipgloss v2 drops AdaptiveColor and
// hands the terminal's background to the program as a message, so the whole
// set has to be rebuildable mid-run.
type styles struct {
	// th is the palette the set was derived from, which the bubbles widgets
	// need in colour form rather than as a style: a real cursor takes a
	// colour, not a foreground.
	th Theme

	// Panes. The focused one differs in shape as well as colour: a thick
	// border survives NO_COLOR and a 16-colour terminal, where the accent
	// alone would not.
	paneBorder        lipgloss.Style
	paneBorderFocused lipgloss.Style
	paneTitle         lipgloss.Style
	paneTitleFocused  lipgloss.Style

	// Text.
	head    lipgloss.Style
	dim     lipgloss.Style
	errText lipgloss.Style
	keyHint lipgloss.Style
	desc    lipgloss.Style
	label   lipgloss.Style
	value   lipgloss.Style

	// Header bar. Every segment carries the bar's background: a nested style
	// ends with a reset, so a segment without one drops the background for the
	// rest of the line.
	headerBar    lipgloss.Style
	headerFill   lipgloss.Style
	headerTitle  lipgloss.Style
	headerAccent lipgloss.Style
	headerDim    lipgloss.Style
	tabActive    lipgloss.Style
	tabIdle      lipgloss.Style
	tabCount     lipgloss.Style

	// The footer's toast, which sits on the terminal's own background rather
	// than on a bar of its own.
	toastInfo    lipgloss.Style
	toastSuccess lipgloss.Style
	toastWarn    lipgloss.Style
	toastError   lipgloss.Style
	toastAccent  lipgloss.Style

	// Modals. Modal text carries the modal's own background for the same
	// reason the status bar's does.
	helpModal  lipgloss.Style
	modalBg    lipgloss.Style
	modalTitle lipgloss.Style
	modalHead  lipgloss.Style
	modalKey   lipgloss.Style
	modalDesc  lipgloss.Style
	modalDim   lipgloss.Style
	modalLabel lipgloss.Style
	modalValue lipgloss.Style
	modalError lipgloss.Style

	// The confirm modal's buttons.
	confirmButton lipgloss.Style
	cancelButton  lipgloss.Style
	idleButton    lipgloss.Style

	// Sections and their contents.
	timerDisplay   lipgloss.Style
	timerLabel     lipgloss.Style
	progressFilled lipgloss.Style
	progressEmpty  lipgloss.Style
	statsBarFilled lipgloss.Style
	statsBarEmpty  lipgloss.Style
	goldBar        lipgloss.Style
	oliveBar       lipgloss.Style

	// Text inputs and the spinner.
	inputPrompt lipgloss.Style
	inputText   lipgloss.Style
	spinner     lipgloss.Style

	// The shared list delegate: title over description, the selection marked
	// by a bar in the accent colour.
	listNormalTitle   lipgloss.Style
	listNormalDesc    lipgloss.Style
	listSelectedTitle lipgloss.Style
	listSelectedDesc  lipgloss.Style
	listDimmedTitle   lipgloss.Style
	listDimmedDesc    lipgloss.Style

	// The activity ramp, from an empty day to the busiest one.
	heat0 lipgloss.Style
	heat1 lipgloss.Style
	heat2 lipgloss.Style
	heat3 lipgloss.Style
	heat4 lipgloss.Style
}

// newStyles derives every style from th. It is called once per Model.
func newStyles(th Theme) styles {
	headerFill := lipgloss.NewStyle().Background(th.Surface)
	modalBg := lipgloss.NewStyle().Background(th.Surface)

	return styles{
		th: th,

		paneBorder:        lipgloss.NewStyle().Foreground(th.Border),
		paneBorderFocused: lipgloss.NewStyle().Foreground(th.BorderFocused),
		paneTitle:         lipgloss.NewStyle().Bold(true).Foreground(th.TextMuted),
		paneTitleFocused:  lipgloss.NewStyle().Bold(true).Foreground(th.Accent),

		head:    lipgloss.NewStyle().Bold(true).Foreground(th.Heading),
		dim:     lipgloss.NewStyle().Foreground(th.TextDim),
		errText: lipgloss.NewStyle().Foreground(th.Error).Bold(true),
		keyHint: lipgloss.NewStyle().Bold(true).Foreground(th.Accent),
		desc:    lipgloss.NewStyle().Foreground(th.TextMuted),
		label:   lipgloss.NewStyle().Foreground(th.TextDim).Bold(true),
		value:   lipgloss.NewStyle().Foreground(th.Text),

		headerBar: lipgloss.NewStyle().
			Background(th.Surface).
			Foreground(th.Text),
		headerFill:   headerFill,
		headerTitle:  headerFill.Bold(true).Foreground(th.Heading),
		headerAccent: headerFill.Bold(true).Foreground(th.Accent),
		headerDim:    headerFill.Foreground(th.TextDim),
		tabActive:    headerFill.Bold(true).Foreground(th.Accent),
		tabIdle:      headerFill.Foreground(th.TextMuted),
		tabCount:     headerFill.Foreground(th.TextDim),

		toastInfo:    lipgloss.NewStyle().Foreground(th.Text),
		toastSuccess: lipgloss.NewStyle().Foreground(th.Success),
		toastWarn:    lipgloss.NewStyle().Foreground(th.Warning),
		toastError:   lipgloss.NewStyle().Bold(true).Foreground(th.Error),
		toastAccent:  lipgloss.NewStyle().Bold(true).Foreground(th.Accent),

		helpModal: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(th.BorderFocused).
			Background(th.Surface).
			Padding(1, 2).
			Width(helpModalWidth),
		modalBg:    modalBg,
		modalTitle: modalBg.Bold(true).Foreground(th.Heading),
		modalHead:  modalBg.Bold(true).Foreground(th.Heading),
		modalKey:   modalBg.Bold(true).Foreground(th.Accent),
		modalDesc:  modalBg.Foreground(th.TextMuted),
		modalDim:   modalBg.Foreground(th.TextDim),
		modalLabel: modalBg.Bold(true).Foreground(th.TextDim),
		modalValue: modalBg.Foreground(th.Text),
		modalError: modalBg.Bold(true).Foreground(th.Error),

		confirmButton: lipgloss.NewStyle().
			Foreground(th.Surface).
			Background(th.Error).
			Bold(true).
			Padding(0, 2),
		cancelButton: lipgloss.NewStyle().
			Foreground(th.Surface).
			Background(th.Success).
			Bold(true).
			Padding(0, 2),
		idleButton: lipgloss.NewStyle().
			Foreground(th.TextMuted).
			Background(th.Surface).
			Padding(0, 2),

		timerDisplay: lipgloss.NewStyle().
			Bold(true).
			Foreground(th.Heading),
		timerLabel: lipgloss.NewStyle().
			Foreground(th.TextDim),
		progressFilled: lipgloss.NewStyle().
			Foreground(th.Success),
		progressEmpty: lipgloss.NewStyle().
			Foreground(th.Border),
		statsBarFilled: lipgloss.NewStyle().
			Foreground(th.Heat3),
		statsBarEmpty: lipgloss.NewStyle().
			Foreground(th.Border),
		goldBar: lipgloss.NewStyle().
			Foreground(th.Accent),
		oliveBar: lipgloss.NewStyle().
			Foreground(th.Success),

		inputPrompt: lipgloss.NewStyle().Foreground(th.Accent).Bold(true),
		inputText:   lipgloss.NewStyle().Foreground(th.Text),
		spinner:     lipgloss.NewStyle().Foreground(th.Accent),

		listNormalTitle: lipgloss.NewStyle().
			Foreground(th.Text).
			Padding(0, 0, 0, 2),
		listNormalDesc: lipgloss.NewStyle().
			Foreground(th.TextMuted).
			Padding(0, 0, 0, 2),
		listSelectedTitle: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(th.Accent).
			Foreground(th.Accent).
			Bold(true).
			Padding(0, 0, 0, 1),
		listSelectedDesc: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(th.Accent).
			Foreground(th.TextMuted).
			Padding(0, 0, 0, 1),
		listDimmedTitle: lipgloss.NewStyle().
			Foreground(th.TextDim).
			Padding(0, 0, 0, 2),
		listDimmedDesc: lipgloss.NewStyle().
			Foreground(th.Border).
			Padding(0, 0, 0, 2),

		heat0: lipgloss.NewStyle().Foreground(th.Border),
		heat1: lipgloss.NewStyle().Foreground(th.Heat1),
		heat2: lipgloss.NewStyle().Foreground(th.Heat2),
		heat3: lipgloss.NewStyle().Foreground(th.Heat3),
		heat4: lipgloss.NewStyle().Foreground(th.Heat4),
	}
}

// ── Theme setting ───────────────────────────────────────────────────────────

// pinnedDark is what the `theme` config key answered, when it answered:
// nil for "auto", which leaves the terminal to be asked.
var pinnedDark *bool

// pinnedPalette is the named palette the `theme` config key chose, nil for
// "auto", "dark" and "light", which pick a background rather than a palette.
var pinnedPalette *NamedTheme

// ApplyThemeSetting honours the `theme` config key. "auto" (or empty) leaves
// the background to be detected — the dashboard asks the terminal for it and
// the CLI queries it once at startup; "dark" and "light" pin it, for terminals
// that do not answer the query or answer it wrongly. Any of the named
// palettes (see NamedThemes) pins the whole palette, and with it the
// background the palette was drawn for.
func ApplyThemeSetting(setting string) error {
	switch name := normalizeThemeName(setting); name {
	case "", "auto":
		pinnedDark, pinnedPalette = nil, nil
	case "dark":
		v := true
		pinnedDark, pinnedPalette = &v, nil
	case "light":
		v := false
		pinnedDark, pinnedPalette = &v, nil
	default:
		nt, ok := lookupNamedTheme(name)
		if !ok {
			return fmt.Errorf("invalid theme %q in config (valid: %s)", setting, strings.Join(ThemeSettings(), ", "))
		}
		v := nt.IsDark
		pinnedDark, pinnedPalette = &v, &nt
	}
	return nil
}

// ActiveTheme is the palette to draw with: the named one the `theme` config
// key chose, or the built-in palette resolved for the background isDark. It
// is what the dashboard and the CLI build their styles from.
func ActiveTheme(isDark bool) Theme {
	if pinnedPalette != nil {
		return pinnedPalette.Theme
	}
	return NewTheme(isDark)
}

// ActiveThemeName is the name of the palette in force, empty unless the
// `theme` config key named one. The help modal shows it, so a reader can
// tell which scheme they are looking at.
func ActiveThemeName() string {
	if pinnedPalette == nil {
		return ""
	}
	return pinnedPalette.Name
}

// PinnedDark reports the background the `theme` config key pinned, if it
// pinned one. The dashboard skips the terminal query when it did, and the CLI
// skips its own detection.
func PinnedDark() (isDark bool, pinned bool) {
	if pinnedDark == nil {
		return false, false
	}
	return *pinnedDark, true
}

// ── Widget styles ───────────────────────────────────────────────────────────

// textInputStyles fills a v2 textinput.Styles from the three styles v1 set
// through PromptStyle, TextStyle and PlaceholderStyle. Both focus states get
// the same look: every input in the dashboard is drawn only while it has the
// keyboard, so a blurred palette would never be seen.
func (s styles) textInputStyles(prompt, text, placeholder lipgloss.Style) textinput.Styles {
	state := textinput.StyleState{
		Prompt:      prompt,
		Text:        text,
		Placeholder: placeholder,
		Suggestion:  placeholder,
	}
	return textinput.Styles{Focused: state, Blurred: state, Cursor: s.cursorStyle()}
}

// textAreaStyles is textInputStyles for the review modal's body field. base
// carries the panel's background, which every row of the field needs or the
// modal ends up striped.
func (s styles) textAreaStyles(base, prompt, text, placeholder lipgloss.Style) textarea.Styles {
	state := textarea.StyleState{
		Base:        base,
		Text:        text,
		Placeholder: placeholder,
		Prompt:      prompt,
		CursorLine:  base,
		EndOfBuffer: base,
	}
	return textarea.Styles{
		Focused: state,
		Blurred: state,
		Cursor:  textarea.CursorStyle(s.cursorStyle()),
	}
}

// cursorStyle is the terminal's own cursor, in the accent colour. The inputs
// draw no block of their own — see SetVirtualCursor(false) — so this is the
// shape and colour the terminal is asked for.
func (s styles) cursorStyle() textinput.CursorStyle {
	return textinput.CursorStyle{Color: s.th.Accent, Shape: tea.CursorBlock, Blink: true}
}
