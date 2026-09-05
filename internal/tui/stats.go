package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/charts"
	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m Model) handleStatsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	switch {
	case key.Matches(msg, k.Down):
		content := m.statsView(m.rightPanelContentWidth())
		_, m.statsScroll = clipLines(content, m.statsScroll+1, m.rightPanelContentHeight())
		return m, nil
	case key.Matches(msg, k.Up):
		if m.statsScroll > 0 {
			m.statsScroll--
		}
		return m, nil
	case key.Matches(msg, k.ScrollTop):
		m.statsScroll = 0
		return m, nil
	case key.Matches(msg, k.Refresh):
		return m.startOp(loadLocalDataCmd(m.app))
	case key.Matches(msg, k.Sync):
		return m.startOp(syncAllAndReloadCmd(m.ctx, m.app))
	case key.Matches(msg, k.NextSection):
		m.statsScroll = 0
		m.nextSection()
		return m, nil
	case key.Matches(msg, k.PrevSection):
		m.statsScroll = 0
		m.prevSection()
		return m, nil
	}
	return m.handleGenericKeys(msg)
}

// ── Chart decoration ───────────────────────────────────────────────────────

// renderStatLine renders value/label pairs on one line:
//
//	"12 books   1,748 pages   ★ 3.9 avg"
func renderStatLine(pairs [][2]string, st styles) string {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, st.timerDisplay.Render(p[0])+" "+st.dim.Render(p[1]))
	}
	return "  " + strings.Join(parts, "   ")
}

// paint turns a lipgloss style into the single-string decorator charts want.
func paint(style lipgloss.Style) func(string) string {
	return func(s string) string { return style.Render(s) }
}

// barPalette colours a bar chart; only the filled run differs between charts.
func (st styles) barPalette(filled lipgloss.Style) charts.Palette {
	return charts.Palette{
		Fill:  paint(filled),
		Empty: paint(st.statsBarEmpty),
		Dim:   paint(st.dim),
	}
}

// heatPalette colours the activity heatmap with the theme's four-step ramp.
func (st styles) heatPalette() charts.Palette {
	return charts.Palette{Cell: st.heatCell, Dim: paint(st.dim)}
}

// heatCell renders an intensity level with the theme's activity ramp.
func (st styles) heatCell(level int) string {
	switch level {
	case 4:
		return st.heat4.Render("█")
	case 3:
		return st.heat3.Render("▓")
	case 2:
		return st.heat2.Render("▒")
	case 1:
		return st.heat1.Render("░")
	default:
		return st.heat0.Render("·")
	}
}

// renderHeatmapTUI returns a heatmap string for the TUI (no printing to
// stdout), narrowed to the weeks the pane can actually draw.
func renderHeatmapTUI(activities []model.DayActivity, weeks, availWidth int, st styles) string {
	return charts.Heatmap(activities, charts.FitHeatmapWeeks(weeks, availWidth), st.heatPalette())
}

// clipLines returns at most height lines of s starting at offset, clamping the
// offset so the last page stays full. The second return is the clamped offset.
func clipLines(s string, offset, height int) (string, int) {
	lines := strings.Split(s, "\n")
	if height <= 0 || len(lines) <= height {
		return s, 0
	}
	maxOffset := len(lines) - height
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	return strings.Join(lines[offset:offset+height], "\n"), offset
}

// statsView renders the unified reading statistics right panel: Hardcover
// library stats (year summary, goal, heatmap, months, ratings, genres) plus
// local timer stats for the current week.
func (m Model) statsView(w int) string {
	rs := m.readingStats

	var sb strings.Builder
	title := "Reading Stats"
	if rs != nil {
		title = fmt.Sprintf("Reading Stats · %d", rs.Year.Year)
	}
	sb.WriteString(m.st.head.Render(title))
	sb.WriteString("\n\n")

	if rs == nil {
		sb.WriteString(m.st.dim.Render("  No stats yet. Press s to sync with Hardcover."))
		return sb.String()
	}

	// Year at a glance.
	pairs := [][2]string{
		{fmt.Sprintf("%d", rs.Year.BooksFinished), "books"},
		{format.Thousands(rs.Year.PagesRead), "pages"},
	}
	if rs.Year.AvgRating > 0 {
		pairs = append(pairs, [2]string{fmt.Sprintf("★ %.1f", rs.Year.AvgRating), "avg"})
	}
	sb.WriteString(renderStatLine(pairs, m.st))
	sb.WriteString("\n\n")

	// Reading goal.
	if g := rs.Goal; g != nil && g.Target > 0 {
		goalLabel := fmt.Sprintf("Goal · %d %s", g.Target, g.Metric)
		if !g.EndDate.IsZero() {
			goalLabel += " by " + g.EndDate.Format("Jan 2")
		}
		sb.WriteString(m.st.label.Render("  " + goalLabel))
		sb.WriteString("\n")
		barW := clampInt(w-14, 10, 30)
		sb.WriteString("  " + progressBar(int(g.Progress), g.Target, barW, m.st))
		sb.WriteString(m.st.dim.Render(fmt.Sprintf("  %d/%d", int(g.Progress), g.Target)))
		sb.WriteString("\n\n")
	}

	// Activity heatmap.
	if len(rs.Heatmap) > 0 {
		sb.WriteString(m.st.label.Render("  Activity"))
		sb.WriteString("\n")
		sb.WriteString(renderHeatmapTUI(rs.Heatmap, 26, w, m.st))
		sb.WriteString("\n\n")
	}

	// Books per month next to ratings distribution when width allows.
	monthChart := m.monthsChart(rs)
	ratingChart := m.ratingsChart(rs)
	sb.WriteString(joinChartsResponsive(w, monthChart, ratingChart, m.st))

	// Books per year next to top genres.
	yearChart := m.yearsChart(rs)
	genreChart := m.genresChart(rs)
	sb.WriteString(joinChartsResponsive(w, yearChart, genreChart, m.st))

	// Timer week.
	sb.WriteString(m.weeklyTimerBlock(w))

	return strings.TrimRight(sb.String(), "\n")
}

// chartBlock is a titled chart fragment used by the stats layout.
type chartBlock struct {
	title string
	body  string
}

// joinChartsResponsive lays two chart blocks side by side when width allows,
// stacked otherwise. Empty blocks are skipped.
func joinChartsResponsive(w int, left, right chartBlock, st styles) string {
	render := func(b chartBlock) string {
		return st.label.Render("  "+b.title) + "\n" + b.body
	}
	switch {
	case left.body == "" && right.body == "":
		return ""
	case left.body == "":
		return render(right) + "\n\n"
	case right.body == "":
		return render(left) + "\n\n"
	}

	leftW := lipgloss.Width(render(left))
	rightW := lipgloss.Width(render(right))
	if leftW+rightW+4 <= w {
		return lipgloss.JoinHorizontal(lipgloss.Top, render(left), "    ", render(right)) + "\n\n"
	}
	return render(left) + "\n\n" + render(right) + "\n\n"
}

func (m Model) monthsChart(rs *model.ReadingStats) chartBlock {
	upto := 12
	if rs.Year.Year == time.Now().Year() {
		upto = int(time.Now().Month())
	}
	rows := make([]model.LabelCount, 0, upto)
	for i := 0; i < upto; i++ {
		rows = append(rows, model.LabelCount{
			Label: time.Month(i + 1).String()[:3],
			Count: rs.Months[i],
		})
	}
	return chartBlock{"Books per month", charts.BarChartH(rows, 3, 10, m.st.barPalette(m.st.statsBarFilled))}
}

func (m Model) ratingsChart(rs *model.ReadingStats) chartBlock {
	rows := make([]model.LabelCount, 0, 10)
	for i := 9; i >= 0; i-- {
		if rs.Ratings[i] == 0 {
			continue
		}
		rows = append(rows, model.LabelCount{
			Label: fmt.Sprintf("★%.1f", float64(i+1)/2),
			Count: rs.Ratings[i],
		})
	}
	if len(rows) == 0 {
		return chartBlock{}
	}
	return chartBlock{"Ratings", charts.BarChartH(rows, 4, 10, m.st.barPalette(m.st.goldBar))}
}

func (m Model) yearsChart(rs *model.ReadingStats) chartBlock {
	if len(rs.Years) < 2 {
		return chartBlock{}
	}
	rows := rs.Years
	if len(rows) > 6 {
		rows = rows[len(rows)-6:]
	}
	return chartBlock{"Books per year", charts.BarChartH(rows, 4, 10, m.st.barPalette(m.st.statsBarFilled))}
}

func (m Model) genresChart(rs *model.ReadingStats) chartBlock {
	if len(rs.Genres) == 0 {
		return chartBlock{}
	}
	labelW := 0
	rows := make([]model.LabelCount, 0, len(rs.Genres))
	for _, g := range rs.Genres {
		label := g.Label
		if len(label) > 14 {
			label = label[:13] + "…"
		}
		if len(label) > labelW {
			labelW = len(label)
		}
		rows = append(rows, model.LabelCount{Label: label, Count: g.Count})
	}
	return chartBlock{"Top genres", charts.BarChartH(rows, labelW, 10, m.st.barPalette(m.st.oliveBar))}
}

// weeklyTimerBlock renders this week's timer minutes as day bars.
func (m Model) weeklyTimerBlock(w int) string {
	if m.weeklyStats.Sessions == 0 {
		return m.st.dim.Render("  No timer sessions this week — press t in Timer to track time.")
	}

	var sb strings.Builder
	sb.WriteString(m.st.label.Render("  This week"))
	sb.WriteString("\n")

	dayNames := [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	maxMin := 1
	for _, mins := range m.weeklyStats.Days {
		if mins > maxMin {
			maxMin = mins
		}
	}
	barWidth := clampInt(w-16, 10, 30)

	for i, mins := range m.weeklyStats.Days {
		filled := mins * barWidth / maxMin
		if mins > 0 && filled == 0 {
			filled = 1
		}
		bar := m.st.statsBarFilled.Render(strings.Repeat("█", filled)) +
			m.st.statsBarEmpty.Render(strings.Repeat("░", barWidth-filled))
		timeStr := "    —"
		if mins > 0 {
			timeStr = fmt.Sprintf("%5s", format.Duration(time.Duration(mins)*time.Minute))
		}
		sb.WriteString(fmt.Sprintf("  %s  %s %s\n", dayNames[i], bar, m.st.dim.Render(timeStr)))
	}

	avg := m.weeklyStats.Total / max(1, m.weeklyStats.Sessions)
	sb.WriteString(fmt.Sprintf("\n  Total: %s    Avg: %s    Sessions: %d",
		format.Duration(time.Duration(m.weeklyStats.Total)*time.Minute),
		format.Duration(time.Duration(avg)*time.Minute),
		m.weeklyStats.Sessions,
	))
	return sb.String()
}
