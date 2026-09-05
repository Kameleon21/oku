package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Theme ───────────────────────────────────────────────────────────────────

// Theme names the colours the dashboard and the CLI output are drawn with.
// Every colour is adaptive: lipgloss picks the Light or the Dark value when
// it renders, from the terminal's reported background (or from the `theme`
// config key, see ApplyThemeSetting). The palette is warm on a dark terminal
// and the same hues darkened on a light one, so nothing washes out.
type Theme struct {
	Accent        lipgloss.AdaptiveColor // focus, key hints, selection
	Heading       lipgloss.AdaptiveColor // titles
	Text          lipgloss.AdaptiveColor // body text
	TextMuted     lipgloss.AdaptiveColor // descriptions, secondary text
	TextDim       lipgloss.AdaptiveColor // hints, counts, subtle text
	Border        lipgloss.AdaptiveColor // unfocused borders, empty tracks
	BorderFocused lipgloss.AdaptiveColor // focused borders
	Surface       lipgloss.AdaptiveColor // status bar and modal background
	Success       lipgloss.AdaptiveColor // done, progress filled
	Warning       lipgloss.AdaptiveColor // wait, retry
	Error         lipgloss.AdaptiveColor // failures, destructive
	Heat1         lipgloss.AdaptiveColor // activity ramp, lightest
	Heat2         lipgloss.AdaptiveColor
	Heat3         lipgloss.AdaptiveColor
	Heat4         lipgloss.AdaptiveColor // activity ramp, busiest
}

// DefaultTheme is the built-in palette, in 256-colour indices. The dark side
// is the gold-and-olive look the dashboard has always had; the light side
// keeps the hues but drops their luminance so they read on white.
func DefaultTheme() Theme {
	return Theme{
		Accent:        lipgloss.AdaptiveColor{Dark: "179", Light: "130"},
		Heading:       lipgloss.AdaptiveColor{Dark: "223", Light: "235"},
		Text:          lipgloss.AdaptiveColor{Dark: "252", Light: "236"},
		TextMuted:     lipgloss.AdaptiveColor{Dark: "245", Light: "242"},
		TextDim:       lipgloss.AdaptiveColor{Dark: "243", Light: "245"},
		Border:        lipgloss.AdaptiveColor{Dark: "238", Light: "250"},
		BorderFocused: lipgloss.AdaptiveColor{Dark: "179", Light: "130"},
		Surface:       lipgloss.AdaptiveColor{Dark: "236", Light: "254"},
		Success:       lipgloss.AdaptiveColor{Dark: "107", Light: "64"},
		Warning:       lipgloss.AdaptiveColor{Dark: "215", Light: "166"},
		Error:         lipgloss.AdaptiveColor{Dark: "167", Light: "160"},
		// A four-step ramp that stays visible at both ends: the dark side
		// climbs from a muted green to a pale one, the light side from a
		// pale green down to a deep one.
		Heat1: lipgloss.AdaptiveColor{Dark: "65", Light: "150"},
		Heat2: lipgloss.AdaptiveColor{Dark: "71", Light: "107"},
		Heat3: lipgloss.AdaptiveColor{Dark: "113", Light: "64"},
		Heat4: lipgloss.AdaptiveColor{Dark: "156", Light: "22"},
	}
}

// ── Styles ──────────────────────────────────────────────────────────────────

// styles is every style the dashboard draws with, derived from one Theme. It
// is a value on the Model rather than a set of package vars because the theme
// is about to become a runtime answer: lipgloss v2 drops AdaptiveColor and
// hands the terminal's background to the program as a message, so the whole
// set has to be rebuildable mid-run.
type styles struct {
	// Panels. The focused one differs in shape as well as colour: a thick
	// border survives NO_COLOR and a 16-colour terminal, where the accent
	// alone would not.
	paneFocused lipgloss.Style
	pane        lipgloss.Style

	// Text.
	head    lipgloss.Style
	dim     lipgloss.Style
	errText lipgloss.Style
	keyHint lipgloss.Style
	desc    lipgloss.Style
	label   lipgloss.Style
	value   lipgloss.Style

	// Status bar. Every segment carries the bar's background: a nested style
	// ends with a reset, so a segment without one drops the background for the
	// rest of the line.
	statusBar        lipgloss.Style
	statusBarFill    lipgloss.Style
	statusBarTitle   lipgloss.Style
	statusBarAccent  lipgloss.Style
	statusBarInfo    lipgloss.Style
	statusBarSuccess lipgloss.Style
	statusBarWarn    lipgloss.Style
	statusBarError   lipgloss.Style

	// listHeader titles a list that has no section card of its own.
	listHeader lipgloss.Style

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
	sectionLabel        lipgloss.Style
	sectionLabelFocused lipgloss.Style
	sectionCountLabel   lipgloss.Style
	timerDisplay        lipgloss.Style
	timerLabel          lipgloss.Style
	progressFilled      lipgloss.Style
	progressEmpty       lipgloss.Style
	statsBarFilled      lipgloss.Style
	statsBarEmpty       lipgloss.Style
	goldBar             lipgloss.Style
	oliveBar            lipgloss.Style

	// The activity ramp, from an empty day to the busiest one.
	heat0 lipgloss.Style
	heat1 lipgloss.Style
	heat2 lipgloss.Style
	heat3 lipgloss.Style
	heat4 lipgloss.Style
}

// newStyles derives every style from th. It is called once per Model.
func newStyles(th Theme) styles {
	statusBarFill := lipgloss.NewStyle().Background(th.Surface)
	modalBg := lipgloss.NewStyle().Background(th.Surface)

	return styles{
		paneFocused: lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(th.BorderFocused),
		pane: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(th.Border),

		head:    lipgloss.NewStyle().Bold(true).Foreground(th.Heading),
		dim:     lipgloss.NewStyle().Foreground(th.TextDim),
		errText: lipgloss.NewStyle().Foreground(th.Error).Bold(true),
		keyHint: lipgloss.NewStyle().Bold(true).Foreground(th.Accent),
		desc:    lipgloss.NewStyle().Foreground(th.TextMuted),
		label:   lipgloss.NewStyle().Foreground(th.TextDim).Bold(true),
		value:   lipgloss.NewStyle().Foreground(th.Text),

		statusBar: lipgloss.NewStyle().
			Background(th.Surface).
			Foreground(th.Text).
			Padding(0, 1),
		statusBarFill:    statusBarFill,
		statusBarTitle:   statusBarFill.Bold(true).Foreground(th.Heading),
		statusBarAccent:  statusBarFill.Bold(true).Foreground(th.Accent),
		statusBarInfo:    statusBarFill.Foreground(th.Text),
		statusBarSuccess: statusBarFill.Foreground(th.Success),
		statusBarWarn:    statusBarFill.Foreground(th.Warning),
		statusBarError:   statusBarFill.Bold(true).Foreground(th.Error),

		listHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(th.Heading).
			Background(th.Surface).
			Padding(0, 1),

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

		sectionLabel: lipgloss.NewStyle().
			Bold(true).
			Foreground(th.TextMuted).
			Padding(0, 0, 0, 1),
		sectionLabelFocused: lipgloss.NewStyle().
			Bold(true).
			Foreground(th.Accent).
			Padding(0, 0, 0, 1),
		sectionCountLabel: lipgloss.NewStyle().
			Foreground(th.TextDim),
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

		heat0: lipgloss.NewStyle().Foreground(th.Border),
		heat1: lipgloss.NewStyle().Foreground(th.Heat1),
		heat2: lipgloss.NewStyle().Foreground(th.Heat2),
		heat3: lipgloss.NewStyle().Foreground(th.Heat3),
		heat4: lipgloss.NewStyle().Foreground(th.Heat4),
	}
}

// ── Theme setting ───────────────────────────────────────────────────────────

// ApplyThemeSetting honours the `theme` config key. "auto" (or empty) leaves
// lipgloss to detect the terminal background; "dark" and "light" pin it, for
// terminals that do not answer the query or answer it wrongly.
func ApplyThemeSetting(setting string) error {
	switch strings.ToLower(strings.TrimSpace(setting)) {
	case "", "auto":
		return nil
	case "dark":
		lipgloss.SetHasDarkBackground(true)
	case "light":
		lipgloss.SetHasDarkBackground(false)
	default:
		return fmt.Errorf("invalid theme %q in config (valid: auto, dark, light)", setting)
	}
	return nil
}
