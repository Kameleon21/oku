package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Color Constants ─────────────────────────────────────────────────────────

var (
	colorGold      = lipgloss.Color("179") // accent, focused borders, key hints
	colorCream     = lipgloss.Color("223") // headers, titles
	colorOlive     = lipgloss.Color("107") // active markers, progress filled
	colorLightGray = lipgloss.Color("252") // normal item titles
	colorMidGray   = lipgloss.Color("245") // descriptions, secondary text
	colorDimGray   = lipgloss.Color("243") // help text, subtle
	colorDarkGray  = lipgloss.Color("238") // unfocused borders, progress empty
	colorCharcoal  = lipgloss.Color("236") // status bar background
	colorGreen     = lipgloss.Color("114") // info / success messages
	colorWarmRed   = lipgloss.Color("167") // error messages
	colorGitHubG1  = lipgloss.Color("#0e4429")
	colorGitHubG2  = lipgloss.Color("#006d32")
	colorGitHubG3  = lipgloss.Color("#26a641")
	colorGitHubG4  = lipgloss.Color("#39d353")
)

// ── Panel Styles ────────────────────────────────────────────────────────────

var (
	panelFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorGold)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorDarkGray)
)

// ── Text Styles ─────────────────────────────────────────────────────────────

var (
	headStyle     = lipgloss.NewStyle().Bold(true).Foreground(colorCream)
	dimStyleTUI   = lipgloss.NewStyle().Foreground(colorDimGray)
	errorStyleTUI = lipgloss.NewStyle().Foreground(colorWarmRed).Bold(true)

	keyStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorGold)
	descStyle  = lipgloss.NewStyle().Foreground(colorMidGray)
	labelStyle = lipgloss.NewStyle().Foreground(colorDimGray).Bold(true)
	valueStyle = lipgloss.NewStyle().Foreground(colorLightGray)

	statusBarStyle = lipgloss.NewStyle().
			Background(colorCharcoal).
			Foreground(colorLightGray).
			Padding(0, 1)

	// Every status-bar segment carries the bar's background. A nested style
	// ends with a reset, so a segment without one drops the background for the
	// rest of the line.
	statusBarFillStyle   = lipgloss.NewStyle().Background(colorCharcoal)
	statusBarTitleStyle  = statusBarFillStyle.Bold(true).Foreground(colorCream)
	statusBarAccentStyle = statusBarFillStyle.Foreground(colorGold)
	statusBarInfoStyle   = statusBarFillStyle.Foreground(colorGreen)
	statusBarErrorStyle  = statusBarFillStyle.Bold(true).Foreground(colorWarmRed)

	// listHeaderStyle titles a list that has no section card of its own.
	listHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCream).
			Background(colorCharcoal).
			Padding(0, 1)
)

// ── Help Modal Style ────────────────────────────────────────────────────────

var helpModalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorGold).
	Background(colorCharcoal).
	Padding(1, 2).
	Width(50)

// Modal text carries the modal's own background. A style that only sets a
// foreground ends its run with a reset, which drops the background for the
// rest of the row and stripes the panel with the terminal's own colour.
var (
	modalBgStyle    = lipgloss.NewStyle().Background(colorCharcoal)
	modalTitleStyle = modalBgStyle.Bold(true).Foreground(colorCream)
	modalHeadStyle  = modalBgStyle.Bold(true).Foreground(colorCream)
	modalKeyStyle   = modalBgStyle.Bold(true).Foreground(colorGold)
	modalDescStyle  = modalBgStyle.Foreground(colorMidGray)
	modalDimStyle   = modalBgStyle.Foreground(colorDimGray)
	modalLabelStyle = modalBgStyle.Bold(true).Foreground(colorDimGray)
	modalValueStyle = modalBgStyle.Foreground(colorLightGray)
	modalErrorStyle = modalBgStyle.Bold(true).Foreground(colorWarmRed)
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
	return style.Render(padModalLines(body, style.GetWidth()-style.GetHorizontalPadding()))
}

// padModalLines repaints the tail of every row with the modal background, so a
// short line does not leave a band of terminal background behind it.
func padModalLines(content string, width int) string {
	if width <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if pad := width - lipgloss.Width(line); pad > 0 {
			lines[i] = line + modalBgStyle.Render(strings.Repeat(" ", pad))
		}
	}
	return strings.Join(lines, "\n")
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

	bar := lipgloss.NewStyle().Foreground(colorOlive).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(colorDarkGray).Render(strings.Repeat("░", empty))

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
				Foreground(colorMidGray).
				Padding(0, 0, 0, 1)

	sectionLabelFocusedStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(colorGold).
					Padding(0, 0, 0, 1)

	sectionCountStyle = lipgloss.NewStyle().
				Foreground(colorDimGray)

	timerDisplayStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorCream)

	timerLabelStyle = lipgloss.NewStyle().
			Foreground(colorDimGray)

	statsBarFilledStyle = lipgloss.NewStyle().
				Foreground(colorGitHubG4)

	statsBarEmptyStyle = lipgloss.NewStyle().
				Foreground(colorDarkGray)

	goldBarStyle = lipgloss.NewStyle().
			Foreground(colorGold)

	oliveBarStyle = lipgloss.NewStyle().
			Foreground(colorOlive)

	heatmapEmptyStyle = lipgloss.NewStyle().
				Foreground(colorDarkGray)

	heatmapLevel1Style = lipgloss.NewStyle().
				Foreground(colorGitHubG1)

	heatmapLevel2Style = lipgloss.NewStyle().
				Foreground(colorGitHubG2)

	heatmapLevel3Style = lipgloss.NewStyle().
				Foreground(colorGitHubG3)

	heatmapLevel4Style = lipgloss.NewStyle().
				Foreground(colorGitHubG4)
)
