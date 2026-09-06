package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Kameleon21/oku/internal/charts"
	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/x/ansi"
)

// statsSection is the reading statistics page: Hardcover library stats
// (year summary, goal, heatmap, months, ratings, genres) plus local timer
// stats for the current week.
//
// The page is drawn into a viewport, so it scrolls a line, half a page or
// the whole way at a time instead of being clipped by hand. Building it is
// a dozen charts' worth of work, so the result is memoised on everything
// that would change it; a data change or a resize drops the memo.
type statsSection struct {
	sh *shared
	st styles
	vp viewport.Model

	// w and h are the pane the page is drawn into.
	w, h int

	// key is what the page in the viewport was built from. The zero value
	// means nothing has been built yet.
	key statsKey
}

// statsKey is everything the page is derived from: a change in any of it is
// a page that has to be built again.
type statsKey struct {
	w       int
	stats   *model.ReadingStats
	weekly  model.WeeklyStats
	density Density
	// built marks a key as a real one, so the zero value never matches.
	built bool
}

func newStatsSection(sh *shared, st styles) *statsSection {
	return &statsSection{sh: sh, st: st, vp: viewport.New(viewport.WithWidth(1), viewport.WithHeight(1))}
}

func (s *statsSection) Update(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(dataChangedMsg); ok {
		// The stats and the timer week come with the local data; the density
		// is not on the page today, but the memo keys on it either way so a
		// row that starts reading it cannot go stale.
		if msg.kind == dataLocal || msg.kind == dataDensity {
			s.key = statsKey{}
		}
		return nil
	}
	if msg, ok := msg.(stylesChangedMsg); ok {
		// The page is drawn with the styles, so the memo has to go with them.
		s.st = msg.st
		s.key = statsKey{}
		return nil
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	// The keys move the viewport, which has to hold the page before it can
	// be scrolled: a key pressed before the first render would otherwise
	// scroll an empty box.
	s.build()
	k := keysFor(s)
	switch {
	case key.Matches(keyMsg, k.Down):
		s.vp.ScrollDown(1)
	case key.Matches(keyMsg, k.Up):
		s.vp.ScrollUp(1)
	case key.Matches(keyMsg, k.HalfPageDown):
		s.vp.HalfPageDown()
	case key.Matches(keyMsg, k.HalfPageUp):
		s.vp.HalfPageUp()
	case key.Matches(keyMsg, k.ScrollTop):
		s.vp.GotoTop()
	case key.Matches(keyMsg, k.ScrollBottom):
		s.vp.GotoBottom()
	case key.Matches(keyMsg, k.Refresh):
		return request(reqRefresh{local: true})
	}
	return nil
}

// build fills the viewport, rebuilding the page only when something it is
// drawn from has changed. A rebuild of the same page keeps the reader where
// they were, clamped in case the page is now shorter.
func (s *statsSection) build() {
	k := statsKey{w: s.w, stats: s.sh.stats, weekly: s.sh.weekly, density: s.sh.density, built: true}
	if k == s.key {
		return
	}
	s.key = k
	offset := s.vp.YOffset()
	s.vp.SetHeight(max(1, s.h))
	s.vp.SetContent(s.render(s.w))
	if s.vp.TotalLineCount() > s.vp.Height() {
		// The badge takes the pane's last row for itself. Stamped over the
		// page it would cover whatever chart happened to be on that row,
		// wherever the reader had scrolled to.
		s.vp.SetHeight(max(1, s.h-1))
	}
	s.vp.SetYOffset(min(offset, max(0, s.vp.TotalLineCount()-s.vp.Height())))
}

// View draws the visible slice of the page, with a scroll indicator on the
// pane's last row when there is more of it than fits.
func (s *statsSection) View(w, h int) string {
	if w != s.w || h != s.h {
		// A View at a size the section has not been resized to is the golden
		// tests' path; the pane is the authority either way.
		s.Resize(w, h)
	}
	s.build()
	// The viewport is a row shorter than the pane when the page overflows,
	// so the row the badge is stamped on is the empty one fitBlock adds.
	out := fitBlock(s.vp.View(), w, h)
	if total := s.vp.TotalLineCount(); total > s.vp.Height() {
		last := min(total, s.vp.YOffset()+s.vp.Height())
		out = stampOverflowBadge(out, fmt.Sprintf("▲ %d/%d ▼", last, total), w, s.st)
	}
	return out
}

func (s *statsSection) Resize(w, h int) tea.Cmd {
	if w == s.w && h == s.h {
		return nil
	}
	s.w, s.h = w, h
	s.vp.SetWidth(max(1, w))
	s.vp.SetHeight(max(1, h))
	// The charts are drawn to the width they are given, so the page itself
	// changes with the pane.
	s.key = statsKey{}
	return nil
}

func (s *statsSection) Keys(k *keyMap) {
	tabHint := hint("tab", k.PrevSection, k.NextSection)
	k.Up.SetHelp("k", "scroll")
	k.Down.SetHelp("j", "scroll")
	enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.ScrollTop, &k.ScrollBottom,
		&k.HalfPageUp, &k.HalfPageDown, &k.NextSection, &k.PrevSection,
		&k.TabJump, &k.Sync, &k.Refresh, &k.Search)
	k.short = []key.Binding{
		k.Help, hint("scroll", k.Down, k.Up), hint("half page", k.HalfPageUp, k.HalfPageDown),
		hintAs("g/G", "top/bottom", k.ScrollTop, k.ScrollBottom), tabHint,
		hintAs("s", "sync", k.Sync), k.Refresh, k.Search, k.Quit,
	}
}

func (s *statsSection) Focus() {}

// Blur forgets the scroll: the page starts at the top when it is next shown.
func (s *statsSection) Blur() { s.vp.GotoTop() }

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
		sb.WriteString("  " + progressBar(int(g.Progress), g.Target, chartBarW(w, 8), st))
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

	// Books per month next to ratings distribution when width allows, and
	// the same for years and genres. A paired block is built at half the
	// pane rather than being built wide and then stacked.
	cw := w
	if paired := pairedChartWidth(w); paired > 0 {
		cw = paired
	}
	sb.WriteString(joinChartsResponsive(w, s.monthsChart(rs, cw), s.ratingsChart(rs, cw), st))
	sb.WriteString(joinChartsResponsive(w, s.yearsChart(rs, cw), s.genresChart(rs, cw), st))

	// Timer week.
	sb.WriteString(s.weeklyTimerBlock(w))

	return strings.TrimRight(sb.String(), "\n")
}

// chartBarW is the track width a bar chart gets in w columns: what is left
// once the label column and the row's own furniture (two of indent, a space
// either side of the bar and the count) have had theirs. The bars used to be
// a fixed ten wide whatever the terminal was.
func chartBarW(w, labelW int) int {
	return clampInt(w-labelW-6, 10, 40)
}

// minPairedChartWidth is the narrowest a chart can be drawn and still say
// something: a label column, a ten-cell bar and the count.
const minPairedChartWidth = 26

// pairedChartWidth is the width each of two side-by-side blocks gets, or 0
// when the pane cannot carry them side by side.
func pairedChartWidth(w int) int {
	if half := (w - 4) / 2; half >= minPairedChartWidth {
		return half
	}
	return 0
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

func (s *statsSection) monthsChart(rs *model.ReadingStats, w int) chartBlock {
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
	return chartBlock{"Books per month", charts.BarChartH(rows, 3, chartBarW(w, 3), s.st.barPalette(s.st.statsBarFilled))}
}

func (s *statsSection) ratingsChart(rs *model.ReadingStats, w int) chartBlock {
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
	return chartBlock{"Ratings", charts.BarChartH(rows, 4, chartBarW(w, 4), s.st.barPalette(s.st.goldBar))}
}

func (s *statsSection) yearsChart(rs *model.ReadingStats, w int) chartBlock {
	if len(rs.Years) < 2 {
		return chartBlock{}
	}
	rows := rs.Years
	if len(rows) > 6 {
		rows = rows[len(rows)-6:]
	}
	return chartBlock{"Books per year", charts.BarChartH(rows, 4, chartBarW(w, 4), s.st.barPalette(s.st.statsBarFilled))}
}

// genreLabelW is the widest a genre label is drawn, ellipsis included.
const genreLabelW = 14

func (s *statsSection) genresChart(rs *model.ReadingStats, w int) chartBlock {
	if len(rs.Genres) == 0 {
		return chartBlock{}
	}
	labelW := 0
	rows := make([]model.LabelCount, 0, len(rs.Genres))
	for _, g := range rs.Genres {
		// Cut by cells, not by bytes: a genre with an accent in it was being
		// sliced mid-rune.
		label := ansi.Truncate(g.Label, genreLabelW, "…")
		if w := ansi.StringWidth(label); w > labelW {
			labelW = w
		}
		rows = append(rows, model.LabelCount{Label: label, Count: g.Count})
	}
	return chartBlock{"Top genres", charts.BarChartH(rows, labelW, chartBarW(w, labelW), s.st.barPalette(s.st.oliveBar))}
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
	barWidth := chartBarW(w, 9)

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
