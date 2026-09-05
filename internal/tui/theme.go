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

// th is the palette every style below is derived from.
var th = DefaultTheme()

// ── Panel Styles ────────────────────────────────────────────────────────────

// The focused panel differs from the others in shape as well as colour: a
// thick border survives NO_COLOR and a 16-colour terminal, where the accent
// alone would not.
var (
	panelFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder()).
				BorderForeground(th.BorderFocused)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(th.Border)
)

// ── Text Styles ─────────────────────────────────────────────────────────────

var (
	headStyle     = lipgloss.NewStyle().Bold(true).Foreground(th.Heading)
	dimStyleTUI   = lipgloss.NewStyle().Foreground(th.TextDim)
	errorStyleTUI = lipgloss.NewStyle().Foreground(th.Error).Bold(true)

	keyStyle   = lipgloss.NewStyle().Bold(true).Foreground(th.Accent)
	descStyle  = lipgloss.NewStyle().Foreground(th.TextMuted)
	labelStyle = lipgloss.NewStyle().Foreground(th.TextDim).Bold(true)
	valueStyle = lipgloss.NewStyle().Foreground(th.Text)

	statusBarStyle = lipgloss.NewStyle().
			Background(th.Surface).
			Foreground(th.Text).
			Padding(0, 1)

	// Every status-bar segment carries the bar's background. A nested style
	// ends with a reset, so a segment without one drops the background for the
	// rest of the line.
	statusBarFillStyle    = lipgloss.NewStyle().Background(th.Surface)
	statusBarTitleStyle   = statusBarFillStyle.Bold(true).Foreground(th.Heading)
	statusBarAccentStyle  = statusBarFillStyle.Bold(true).Foreground(th.Accent)
	statusBarInfoStyle    = statusBarFillStyle.Foreground(th.Text)
	statusBarSuccessStyle = statusBarFillStyle.Foreground(th.Success)
	statusBarWarnStyle    = statusBarFillStyle.Foreground(th.Warning)
	statusBarErrorStyle   = statusBarFillStyle.Bold(true).Foreground(th.Error)

	// listHeaderStyle titles a list that has no section card of its own.
	listHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(th.Heading).
			Background(th.Surface).
			Padding(0, 1)
)

// ── Help Modal Style ────────────────────────────────────────────────────────

var helpModalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(th.BorderFocused).
	Background(th.Surface).
	Padding(1, 2).
	Width(50)

// Modal text carries the modal's own background. A style that only sets a
// foreground ends its run with a reset, which drops the background for the
// rest of the row and stripes the panel with the terminal's own colour.
var (
	modalBgStyle    = lipgloss.NewStyle().Background(th.Surface)
	modalTitleStyle = modalBgStyle.Bold(true).Foreground(th.Heading)
	modalHeadStyle  = modalBgStyle.Bold(true).Foreground(th.Heading)
	modalKeyStyle   = modalBgStyle.Bold(true).Foreground(th.Accent)
	modalDescStyle  = modalBgStyle.Foreground(th.TextMuted)
	modalDimStyle   = modalBgStyle.Foreground(th.TextDim)
	modalLabelStyle = modalBgStyle.Bold(true).Foreground(th.TextDim)
	modalValueStyle = modalBgStyle.Foreground(th.Text)
	modalErrorStyle = modalBgStyle.Bold(true).Foreground(th.Error)
)

// ── Section Styles ─────────────────────────────────────────────────────────

var (
	sectionLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(th.TextMuted).
				Padding(0, 0, 0, 1)

	sectionLabelFocusedStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(th.Accent).
					Padding(0, 0, 0, 1)

	sectionCountStyle = lipgloss.NewStyle().
				Foreground(th.TextDim)

	timerDisplayStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(th.Heading)

	timerLabelStyle = lipgloss.NewStyle().
			Foreground(th.TextDim)

	progressFilledStyle = lipgloss.NewStyle().
				Foreground(th.Success)

	progressEmptyStyle = lipgloss.NewStyle().
				Foreground(th.Border)

	statsBarFilledStyle = lipgloss.NewStyle().
				Foreground(th.Heat3)

	statsBarEmptyStyle = lipgloss.NewStyle().
				Foreground(th.Border)

	goldBarStyle = lipgloss.NewStyle().
			Foreground(th.Accent)

	oliveBarStyle = lipgloss.NewStyle().
			Foreground(th.Success)

	heatmapEmptyStyle = lipgloss.NewStyle().
				Foreground(th.Border)

	heatmapLevel1Style = lipgloss.NewStyle().
				Foreground(th.Heat1)

	heatmapLevel2Style = lipgloss.NewStyle().
				Foreground(th.Heat2)

	heatmapLevel3Style = lipgloss.NewStyle().
				Foreground(th.Heat3)

	heatmapLevel4Style = lipgloss.NewStyle().
				Foreground(th.Heat4)
)

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
