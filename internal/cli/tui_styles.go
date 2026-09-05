package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Theme ───────────────────────────────────────────────────────────────────

// theme names the colours the dashboard and the CLI output are drawn with.
// Every colour is adaptive: lipgloss picks the Light or the Dark value when
// it renders, from the terminal's reported background (or from the `theme`
// config key, see applyThemeSetting). The palette is warm on a dark terminal
// and the same hues darkened on a light one, so nothing washes out.
type theme struct {
	accent        lipgloss.AdaptiveColor // focus, key hints, selection
	heading       lipgloss.AdaptiveColor // titles
	text          lipgloss.AdaptiveColor // body text
	textMuted     lipgloss.AdaptiveColor // descriptions, secondary text
	textDim       lipgloss.AdaptiveColor // hints, counts, subtle text
	border        lipgloss.AdaptiveColor // unfocused borders, empty tracks
	borderFocused lipgloss.AdaptiveColor // focused borders
	surface       lipgloss.AdaptiveColor // status bar and modal background
	success       lipgloss.AdaptiveColor // done, progress filled
	warning       lipgloss.AdaptiveColor // wait, retry
	error         lipgloss.AdaptiveColor // failures, destructive
	heat1         lipgloss.AdaptiveColor // activity ramp, lightest
	heat2         lipgloss.AdaptiveColor
	heat3         lipgloss.AdaptiveColor
	heat4         lipgloss.AdaptiveColor // activity ramp, busiest
}

// defaultTheme is the built-in palette, in 256-colour indices. The dark side
// is the gold-and-olive look the dashboard has always had; the light side
// keeps the hues but drops their luminance so they read on white.
func defaultTheme() theme {
	return theme{
		accent:        lipgloss.AdaptiveColor{Dark: "179", Light: "130"},
		heading:       lipgloss.AdaptiveColor{Dark: "223", Light: "235"},
		text:          lipgloss.AdaptiveColor{Dark: "252", Light: "236"},
		textMuted:     lipgloss.AdaptiveColor{Dark: "245", Light: "242"},
		textDim:       lipgloss.AdaptiveColor{Dark: "243", Light: "245"},
		border:        lipgloss.AdaptiveColor{Dark: "238", Light: "250"},
		borderFocused: lipgloss.AdaptiveColor{Dark: "179", Light: "130"},
		surface:       lipgloss.AdaptiveColor{Dark: "236", Light: "254"},
		success:       lipgloss.AdaptiveColor{Dark: "107", Light: "64"},
		warning:       lipgloss.AdaptiveColor{Dark: "215", Light: "166"},
		error:         lipgloss.AdaptiveColor{Dark: "167", Light: "160"},
		// A four-step ramp that stays visible at both ends: the dark side
		// climbs from a muted green to a pale one, the light side from a
		// pale green down to a deep one.
		heat1: lipgloss.AdaptiveColor{Dark: "65", Light: "150"},
		heat2: lipgloss.AdaptiveColor{Dark: "71", Light: "107"},
		heat3: lipgloss.AdaptiveColor{Dark: "113", Light: "64"},
		heat4: lipgloss.AdaptiveColor{Dark: "156", Light: "22"},
	}
}

// th is the palette every style below is derived from.
var th = defaultTheme()

// ── Panel Styles ────────────────────────────────────────────────────────────

// The focused panel differs from the others in shape as well as colour: a
// thick border survives NO_COLOR and a 16-colour terminal, where the accent
// alone would not.
var (
	panelFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder()).
				BorderForeground(th.borderFocused)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(th.border)
)

// ── Text Styles ─────────────────────────────────────────────────────────────

var (
	headStyle     = lipgloss.NewStyle().Bold(true).Foreground(th.heading)
	dimStyleTUI   = lipgloss.NewStyle().Foreground(th.textDim)
	errorStyleTUI = lipgloss.NewStyle().Foreground(th.error).Bold(true)

	keyStyle   = lipgloss.NewStyle().Bold(true).Foreground(th.accent)
	descStyle  = lipgloss.NewStyle().Foreground(th.textMuted)
	labelStyle = lipgloss.NewStyle().Foreground(th.textDim).Bold(true)
	valueStyle = lipgloss.NewStyle().Foreground(th.text)

	statusBarStyle = lipgloss.NewStyle().
			Background(th.surface).
			Foreground(th.text).
			Padding(0, 1)

	// Every status-bar segment carries the bar's background. A nested style
	// ends with a reset, so a segment without one drops the background for the
	// rest of the line.
	statusBarFillStyle    = lipgloss.NewStyle().Background(th.surface)
	statusBarTitleStyle   = statusBarFillStyle.Bold(true).Foreground(th.heading)
	statusBarAccentStyle  = statusBarFillStyle.Bold(true).Foreground(th.accent)
	statusBarInfoStyle    = statusBarFillStyle.Foreground(th.text)
	statusBarSuccessStyle = statusBarFillStyle.Foreground(th.success)
	statusBarWarnStyle    = statusBarFillStyle.Foreground(th.warning)
	statusBarErrorStyle   = statusBarFillStyle.Bold(true).Foreground(th.error)

	// listHeaderStyle titles a list that has no section card of its own.
	listHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(th.heading).
			Background(th.surface).
			Padding(0, 1)
)

// ── Help Modal Style ────────────────────────────────────────────────────────

var helpModalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(th.borderFocused).
	Background(th.surface).
	Padding(1, 2).
	Width(50)

// Modal text carries the modal's own background. A style that only sets a
// foreground ends its run with a reset, which drops the background for the
// rest of the row and stripes the panel with the terminal's own colour.
var (
	modalBgStyle    = lipgloss.NewStyle().Background(th.surface)
	modalTitleStyle = modalBgStyle.Bold(true).Foreground(th.heading)
	modalHeadStyle  = modalBgStyle.Bold(true).Foreground(th.heading)
	modalKeyStyle   = modalBgStyle.Bold(true).Foreground(th.accent)
	modalDescStyle  = modalBgStyle.Foreground(th.textMuted)
	modalDimStyle   = modalBgStyle.Foreground(th.textDim)
	modalLabelStyle = modalBgStyle.Bold(true).Foreground(th.textDim)
	modalValueStyle = modalBgStyle.Foreground(th.text)
	modalErrorStyle = modalBgStyle.Bold(true).Foreground(th.error)
)

func renderModalPanel(title, content string, width int) string {
	style := helpModalStyle
	if width > 0 {
		style = style.Width(width)
	}

	body := content
	if strings.TrimSpace(title) != "" {
		body = modalTitleStyle.Render(title) + "\n\n" + content
	}
	// The rows are left short on purpose: lipgloss fills them out to the panel
	// width with the style's own background, which is the only fill that
	// carries it. Pre-padding here would be trimmed by the wrap and refilled
	// with unstyled spaces, striping the panel.
	return style.Render(body)
}

// ── Progress Bar ────────────────────────────────────────────────────────────

// progressBar renders a Unicode block-character progress bar.
//
//	progressBar(45, 300, 20) → "███░░░░░░░░░░░░░░░░░  15%"
func progressBar(current, total, width int) string {
	if total <= 0 {
		return dimStyleTUI.Render(fmt.Sprintf("p.%d", current))
	}
	pct := float64(current) / float64(total)
	if pct > 1.0 {
		pct = 1.0
	}
	if pct < 0 {
		pct = 0
	}
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	bar := progressFilledStyle.Render(strings.Repeat("█", filled)) +
		progressEmptyStyle.Render(strings.Repeat("░", empty))

	pctStr := fmt.Sprintf("%3d%%", int(pct*100))
	return fmt.Sprintf("%s %s", bar, dimStyleTUI.Render(pctStr))
}

// miniProgressBar renders a compact progress bar for inline list items.
func miniProgressBar(current, total, width int) string {
	if total <= 0 {
		return ""
	}
	pct := float64(current) / float64(total)
	if pct > 1.0 {
		pct = 1.0
	}
	if pct < 0 {
		pct = 0
	}
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	return strings.Repeat("█", filled) + strings.Repeat("░", empty) +
		fmt.Sprintf(" %d%%", int(pct*100))
}

// ── Section Styles ─────────────────────────────────────────────────────────

var (
	sectionLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(th.textMuted).
				Padding(0, 0, 0, 1)

	sectionLabelFocusedStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(th.accent).
					Padding(0, 0, 0, 1)

	sectionCountStyle = lipgloss.NewStyle().
				Foreground(th.textDim)

	timerDisplayStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(th.heading)

	timerLabelStyle = lipgloss.NewStyle().
			Foreground(th.textDim)

	progressFilledStyle = lipgloss.NewStyle().
				Foreground(th.success)

	progressEmptyStyle = lipgloss.NewStyle().
				Foreground(th.border)

	statsBarFilledStyle = lipgloss.NewStyle().
				Foreground(th.heat3)

	statsBarEmptyStyle = lipgloss.NewStyle().
				Foreground(th.border)

	goldBarStyle = lipgloss.NewStyle().
			Foreground(th.accent)

	oliveBarStyle = lipgloss.NewStyle().
			Foreground(th.success)

	heatmapEmptyStyle = lipgloss.NewStyle().
				Foreground(th.border)

	heatmapLevel1Style = lipgloss.NewStyle().
				Foreground(th.heat1)

	heatmapLevel2Style = lipgloss.NewStyle().
				Foreground(th.heat2)

	heatmapLevel3Style = lipgloss.NewStyle().
				Foreground(th.heat3)

	heatmapLevel4Style = lipgloss.NewStyle().
				Foreground(th.heat4)
)

// ── Theme setting ───────────────────────────────────────────────────────────

// applyThemeSetting honours the `theme` config key. "auto" (or empty) leaves
// lipgloss to detect the terminal background; "dark" and "light" pin it, for
// terminals that do not answer the query or answer it wrongly.
func applyThemeSetting(setting string) error {
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
