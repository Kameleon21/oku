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

// statsSection is the reading statistics page: Hardcover library stats
// (year summary, goal, heatmap, months, ratings, genres) plus local timer
// stats for the current week, scrolled a line at a time.
type statsSection struct {
	sh     *shared
	st     styles
	scroll int
	// w and h are the pane the page is drawn into, so a scroll can clamp
	// against what is actually visible.
	w, h int
}

func newStatsSection(sh *shared, st styles) *statsSection {
	return &statsSection{sh: sh, st: st}
}

func (s *statsSection) Update(msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	k := keysFor(s)
	switch {
	case key.Matches(keyMsg, k.Down):
		_, s.scroll = clipLines(s.render(s.w), s.scroll+1, s.h)
	case key.Matches(keyMsg, k.Up):
		if s.scroll > 0 {
			s.scroll--
		}
	case key.Matches(keyMsg, k.ScrollTop):
		s.scroll = 0
	case key.Matches(keyMsg, k.Refresh):
		return request(reqRefresh{local: true})
	}
	return nil
}

func (s *statsSection) View(w, h int) string {
	content, _ := clipLines(s.render(w), s.scroll, h)
	return fitBlock(content, w, h)
}

func (s *statsSection) Resize(w, h int) tea.Cmd {
	s.w, s.h = w, h
	return nil
}

func (s *statsSection) Keys(k *keyMap) {
	tabHint := hint("tab", k.PrevSection, k.NextSection)
	k.Up.SetHelp("k", "scroll")
	k.Down.SetHelp("j", "scroll")
	enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.ScrollTop, &k.NextSection, &k.PrevSection,
		&k.TabJump, &k.Sync, &k.Refresh, &k.Search)
	k.short = []key.Binding{
		k.Help, hint("scroll", k.Down, k.Up), k.ScrollTop, tabHint,
		hintAs("s", "sync", k.Sync), k.Refresh, k.Search, k.Quit,
	}
}

func (s *statsSection) Focus() {}

// Blur forgets the scroll: the page starts at the top when it is next shown.
func (s *statsSection) Blur() { s.scroll = 0 }

func (s *statsSection) CapturesKeys() bool { return false }

func (s *statsSection) Title() string {
	if s.sh.stats != nil {
		return fmt.Sprintf("Stats · %d", s.sh.stats.Year.Year)
	}
	return "Stats"
}

func (s *statsSection) Selected() selection { return selection{} }

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
func renderHeatmapTUI(activities []model.DayActivity, weeks, availWidth int, now time.Time, st styles) string {
	return charts.HeatmapAt(activities, charts.FitHeatmapWeeks(weeks, availWidth), now, st.heatPalette())
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

// render draws the whole page at width w.
func (s *statsSection) render(w int) string {
	rs := s.sh.stats
	st := s.st

	// No heading: the pane's own title already names the page and the year.
	var sb strings.Builder
	if rs == nil {
		sb.WriteString(st.dim.Render("  No stats yet. Press s to sync with Hardcover."))
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
	sb.WriteString(renderStatLine(pairs, st))
	sb.WriteString("\n\n")

	// Reading goal.
	if g := rs.Goal; g != nil && g.Target > 0 {
		goalLabel := fmt.Sprintf("Goal · %d %s", g.Target, g.Metric)
		if !g.EndDate.IsZero() {
			goalLabel += " by " + g.EndDate.Format("Jan 2")
		}
		sb.WriteString(st.label.Render("  " + goalLabel))
		sb.WriteString("\n")
		barW := clampInt(w-14, 10, 30)
		sb.WriteString("  " + progressBar(int(g.Progress), g.Target, barW, st))
		sb.WriteString(st.dim.Render(fmt.Sprintf("  %d/%d", int(g.Progress), g.Target)))
		sb.WriteString("\n\n")
	}

	// Activity heatmap.
	if len(rs.Heatmap) > 0 {
		sb.WriteString(st.label.Render("  Activity"))
		sb.WriteString("\n")
		sb.WriteString(renderHeatmapTUI(rs.Heatmap, 26, w, s.sh.now(), st))
		sb.WriteString("\n\n")
	}

	// Books per month next to ratings distribution when width allows.
	monthChart := s.monthsChart(rs)
	ratingChart := s.ratingsChart(rs)
	sb.WriteString(joinChartsResponsive(w, monthChart, ratingChart, st))

	// Books per year next to top genres.
	yearChart := s.yearsChart(rs)
	genreChart := s.genresChart(rs)
	sb.WriteString(joinChartsResponsive(w, yearChart, genreChart, st))

	// Timer week.
	sb.WriteString(s.weeklyTimerBlock(w))

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

func (s *statsSection) monthsChart(rs *model.ReadingStats) chartBlock {
	now := s.sh.now()
	upto := 12
	if rs.Year.Year == now.Year() {
		upto = int(now.Month())
	}
	rows := make([]model.LabelCount, 0, upto)
	for i := 0; i < upto; i++ {
		rows = append(rows, model.LabelCount{
			Label: time.Month(i + 1).String()[:3],
			Count: rs.Months[i],
		})
	}
	return chartBlock{"Books per month", charts.BarChartH(rows, 3, 10, s.st.barPalette(s.st.statsBarFilled))}
}

func (s *statsSection) ratingsChart(rs *model.ReadingStats) chartBlock {
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
	return chartBlock{"Ratings", charts.BarChartH(rows, 4, 10, s.st.barPalette(s.st.goldBar))}
}

func (s *statsSection) yearsChart(rs *model.ReadingStats) chartBlock {
	if len(rs.Years) < 2 {
		return chartBlock{}
	}
	rows := rs.Years
	if len(rows) > 6 {
		rows = rows[len(rows)-6:]
	}
	return chartBlock{"Books per year", charts.BarChartH(rows, 4, 10, s.st.barPalette(s.st.statsBarFilled))}
}

func (s *statsSection) genresChart(rs *model.ReadingStats) chartBlock {
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
	return chartBlock{"Top genres", charts.BarChartH(rows, labelW, 10, s.st.barPalette(s.st.oliveBar))}
}

// weeklyTimerBlock renders this week's timer minutes as day bars.
func (s *statsSection) weeklyTimerBlock(w int) string {
	weekly := s.sh.weekly
	st := s.st
	if weekly.Sessions == 0 {
		return st.dim.Render("  No timer sessions this week — press t in Timer to track time.")
	}

	var sb strings.Builder
	sb.WriteString(st.label.Render("  This week"))
	sb.WriteString("\n")

	dayNames := [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	maxMin := 1
	for _, mins := range weekly.Days {
		if mins > maxMin {
			maxMin = mins
		}
	}
	barWidth := clampInt(w-16, 10, 30)

	for i, mins := range weekly.Days {
		filled := mins * barWidth / maxMin
		if mins > 0 && filled == 0 {
			filled = 1
		}
		bar := st.statsBarFilled.Render(strings.Repeat("█", filled)) +
			st.statsBarEmpty.Render(strings.Repeat("░", barWidth-filled))
		timeStr := "    —"
		if mins > 0 {
			timeStr = fmt.Sprintf("%5s", format.Duration(time.Duration(mins)*time.Minute))
		}
		sb.WriteString(fmt.Sprintf("  %s  %s %s\n", dayNames[i], bar, st.dim.Render(timeStr)))
	}

	avg := weekly.Total / max(1, weekly.Sessions)
	sb.WriteString(fmt.Sprintf("\n  Total: %s    Avg: %s    Sessions: %d",
		format.Duration(time.Duration(weekly.Total)*time.Minute),
		format.Duration(time.Duration(avg)*time.Minute),
		weekly.Sessions,
	))
	return sb.String()
}
