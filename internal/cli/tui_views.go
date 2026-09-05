package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/charts"
	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/lipgloss"
)

// ── Chart decoration ───────────────────────────────────────────────────────

// renderStatLine renders value/label pairs on one line:
//
//	"12 books   1,748 pages   ★ 3.9 avg"
func renderStatLine(pairs [][2]string) string {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, timerDisplayStyle.Render(p[0])+" "+dimStyleTUI.Render(p[1]))
	}
	return "  " + strings.Join(parts, "   ")
}

// paint turns a lipgloss style into the single-string decorator charts want.
func paint(st lipgloss.Style) func(string) string {
	return func(s string) string { return st.Render(s) }
}

// barPalette colours a bar chart; only the filled run differs between charts.
func barPalette(filled lipgloss.Style) charts.Palette {
	return charts.Palette{
		Fill:  paint(filled),
		Empty: paint(statsBarEmptyStyle),
		Dim:   paint(dimStyleTUI),
	}
}

// heatPalette colours the activity heatmap with the theme's four-step ramp.
func heatPalette() charts.Palette {
	return charts.Palette{Cell: heatmapCellStyled, Dim: paint(dimStyleTUI)}
}

// heatmapCellStyled renders an intensity level with TUI colors.
func heatmapCellStyled(level int) string {
	switch level {
	case 4:
		return heatmapLevel4Style.Render("█")
	case 3:
		return heatmapLevel3Style.Render("▓")
	case 2:
		return heatmapLevel2Style.Render("▒")
	case 1:
		return heatmapLevel1Style.Render("░")
	default:
		return heatmapEmptyStyle.Render("·")
	}
}

// renderHeatmapTUI returns a heatmap string for the TUI (no printing to
// stdout), narrowed to the weeks the pane can actually draw.
func renderHeatmapTUI(activities []model.DayActivity, weeks, availWidth int) string {
	return charts.Heatmap(activities, charts.FitHeatmapWeeks(weeks, availWidth), heatPalette())
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

const okuASCII = `
   ____  __ __  __  __
  / __ \/ //_/ / / / /
 / / / / ,<   / / / / 
/ /_/ / /| | / /_/ /  
\____/_/ |_| \____/   
`

// introView renders the intro/welcome right panel.
func (m dashboardModel) introView(w int) string {
	var sb strings.Builder

	sb.WriteString(headStyle.Render(okuASCII))
	sb.WriteString("\n")
	sb.WriteString(dimStyleTUI.Render("  a reading companion"))
	sb.WriteString("\n\n")

	writeField := func(label, value string) {
		sb.WriteString(labelStyle.Render(fmt.Sprintf("  %-10s ", label)))
		sb.WriteString(valueStyle.Render(value))
		sb.WriteString("\n")
	}

	if m.version != "" {
		writeField("Version", m.version)
	}
	writeField("Reading", fmt.Sprintf("%d books", len(m.readingBooks)))
	writeField("Oku", fmt.Sprintf("%d books", len(m.okuBooks)))

	if m.readingStats != nil && m.readingStats.Year.BooksFinished > 0 {
		writeField("This year", fmt.Sprintf("%d books", m.readingStats.Year.BooksFinished))
	}

	if m.timerState != nil {
		elapsed := time.Since(m.timerState.StartedAt)
		bookTitle := ""
		if m.timerBook != nil {
			bookTitle = m.timerBook.Title
		}

		if bookTitle != "" {
			writeField("Timer", fmt.Sprintf("%s (%s)", format.Duration(elapsed), bookTitle))
		} else {
			writeField("Timer", format.Duration(elapsed))
		}
	} else {
		writeField("Timer", "not running")
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyleTUI.Render("  j/k navigate   h/l section   ? help"))

	return sb.String()
}

// statsView renders the unified reading statistics right panel: Hardcover
// library stats (year summary, goal, heatmap, months, ratings, genres) plus
// local timer stats for the current week.
func (m dashboardModel) statsView(w int) string {
	rs := m.readingStats

	var sb strings.Builder
	title := "Reading Stats"
	if rs != nil {
		title = fmt.Sprintf("Reading Stats · %d", rs.Year.Year)
	}
	sb.WriteString(headStyle.Render(title))
	sb.WriteString("\n\n")

	if rs == nil {
		sb.WriteString(dimStyleTUI.Render("  No stats yet. Press s to sync with Hardcover."))
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
	sb.WriteString(renderStatLine(pairs))
	sb.WriteString("\n\n")

	// Reading goal.
	if g := rs.Goal; g != nil && g.Target > 0 {
		goalLabel := fmt.Sprintf("Goal · %d %s", g.Target, g.Metric)
		if !g.EndDate.IsZero() {
			goalLabel += " by " + g.EndDate.Format("Jan 2")
		}
		sb.WriteString(labelStyle.Render("  " + goalLabel))
		sb.WriteString("\n")
		barW := clampInt(w-14, 10, 30)
		sb.WriteString("  " + progressBar(int(g.Progress), g.Target, barW))
		sb.WriteString(dimStyleTUI.Render(fmt.Sprintf("  %d/%d", int(g.Progress), g.Target)))
		sb.WriteString("\n\n")
	}

	// Activity heatmap.
	if len(rs.Heatmap) > 0 {
		sb.WriteString(labelStyle.Render("  Activity"))
		sb.WriteString("\n")
		sb.WriteString(renderHeatmapTUI(rs.Heatmap, 26, w))
		sb.WriteString("\n\n")
	}

	// Books per month next to ratings distribution when width allows.
	monthChart := m.monthsChart(rs)
	ratingChart := m.ratingsChart(rs)
	sb.WriteString(joinChartsResponsive(w, monthChart, ratingChart))

	// Books per year next to top genres.
	yearChart := m.yearsChart(rs)
	genreChart := m.genresChart(rs)
	sb.WriteString(joinChartsResponsive(w, yearChart, genreChart))

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
func joinChartsResponsive(w int, left, right chartBlock) string {
	render := func(b chartBlock) string {
		return labelStyle.Render("  "+b.title) + "\n" + b.body
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

func (m dashboardModel) monthsChart(rs *model.ReadingStats) chartBlock {
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
	return chartBlock{"Books per month", charts.BarChartH(rows, 3, 10, barPalette(statsBarFilledStyle))}
}

func (m dashboardModel) ratingsChart(rs *model.ReadingStats) chartBlock {
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
	return chartBlock{"Ratings", charts.BarChartH(rows, 4, 10, barPalette(goldBarStyle))}
}

func (m dashboardModel) yearsChart(rs *model.ReadingStats) chartBlock {
	if len(rs.Years) < 2 {
		return chartBlock{}
	}
	rows := rs.Years
	if len(rows) > 6 {
		rows = rows[len(rows)-6:]
	}
	return chartBlock{"Books per year", charts.BarChartH(rows, 4, 10, barPalette(statsBarFilledStyle))}
}

func (m dashboardModel) genresChart(rs *model.ReadingStats) chartBlock {
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
	return chartBlock{"Top genres", charts.BarChartH(rows, labelW, 10, barPalette(oliveBarStyle))}
}

// weeklyTimerBlock renders this week's timer minutes as day bars.
func (m dashboardModel) weeklyTimerBlock(w int) string {
	if m.weeklyStats.Sessions == 0 {
		return dimStyleTUI.Render("  No timer sessions this week — press t in Timer to track time.")
	}

	var sb strings.Builder
	sb.WriteString(labelStyle.Render("  This week"))
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
		bar := statsBarFilledStyle.Render(strings.Repeat("█", filled)) +
			statsBarEmptyStyle.Render(strings.Repeat("░", barWidth-filled))
		timeStr := "    —"
		if mins > 0 {
			timeStr = fmt.Sprintf("%5s", format.Duration(time.Duration(mins)*time.Minute))
		}
		sb.WriteString(fmt.Sprintf("  %s  %s %s\n", dayNames[i], bar, dimStyleTUI.Render(timeStr)))
	}

	avg := m.weeklyStats.Total / max(1, m.weeklyStats.Sessions)
	sb.WriteString(fmt.Sprintf("\n  Total: %s    Avg: %s    Sessions: %d",
		format.Duration(time.Duration(m.weeklyStats.Total)*time.Minute),
		format.Duration(time.Duration(avg)*time.Minute),
		m.weeklyStats.Sessions,
	))
	return sb.String()
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// timerView renders the timer right panel.
func (m dashboardModel) timerView(w int) string {
	var sb strings.Builder

	sb.WriteString(headStyle.Render("Reading Timer"))
	sb.WriteString("\n\n")

	if m.timerSelecting && m.timerState == nil {
		sb.WriteString(labelStyle.Render("  Select a book"))
		sb.WriteString("\n")
		sb.WriteString(dimStyleTUI.Render("  j/k move   Enter start   Esc cancel"))
		sb.WriteString("\n\n")

		if len(m.readingBooks) == 0 {
			sb.WriteString(dimStyleTUI.Render("  No books in Reading."))
			return sb.String()
		}

		maxTitle := w - 8
		if maxTitle < 12 {
			maxTitle = 12
		}
		for i, b := range m.readingBooks {
			if i >= 9 {
				break
			}
			title := b.Book.Title
			if len(title) > maxTitle {
				title = title[:maxTitle-3] + "..."
			}
			author := b.Book.AuthorString()
			if author == "" {
				author = "Unknown author"
			}

			prefix := "  "
			titleStyle := valueStyle
			if i == m.timerSelectIdx {
				prefix = "▸ "
				titleStyle = keyStyle
			}

			sb.WriteString(titleStyle.Render(prefix + title))
			sb.WriteString("\n")
			sb.WriteString(dimStyleTUI.Render("  " + author))
			sb.WriteString("\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	if m.timerState == nil {
		sb.WriteString(dimStyleTUI.Render("  No timer running."))
		sb.WriteString("\n\n")
		sb.WriteString(dimStyleTUI.Render("  Press [t] to choose a book, or [t] in Reading."))
	} else {
		// Book info.
		if m.timerBook != nil {
			sb.WriteString(valueStyle.Render("  " + m.timerBook.Title))
			sb.WriteString("\n")
			if author := m.timerBook.AuthorString(); author != "" {
				sb.WriteString(dimStyleTUI.Render("  " + author))
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}

		// Large timer display.
		elapsed := time.Since(m.timerState.StartedAt)
		h := int(elapsed.Hours())
		min := int(elapsed.Minutes()) % 60
		sec := int(elapsed.Seconds()) % 60
		timeStr := fmt.Sprintf("%02d:%02d:%02d", h, min, sec)

		sb.WriteString(timerDisplayStyle.Render(fmt.Sprintf("       %s", timeStr)))
		sb.WriteString("\n")
		sb.WriteString(timerLabelStyle.Render("        elapsed"))
		sb.WriteString("\n\n")

		sb.WriteString(dimStyleTUI.Render(fmt.Sprintf("  Started: %s",
			m.timerState.StartedAt.Local().Format("3:04 PM"))))
	}

	// Today's stats.
	todayCount := 0
	todayMinutes := 0
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for _, s := range m.recentSessions {
		started := s.StartedAt.Local()
		sessionDate := time.Date(started.Year(), started.Month(), started.Day(), 0, 0, 0, 0, started.Location())
		if sessionDate.Equal(today) {
			todayCount++
			todayMinutes += int(s.Duration().Minutes())
		}
	}
	if todayCount > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("  Today  %d sessions  %s total",
			todayCount, format.Duration(time.Duration(todayMinutes)*time.Minute)))
	}

	// Recent sessions.
	if len(m.recentSessions) > 0 {
		sb.WriteString("\n")
		sb.WriteString(dimStyleTUI.Render("  ────────────────────────────────"))
		sb.WriteString("\n")
		sb.WriteString(labelStyle.Render("  Recent"))
		sb.WriteString("\n")

		yesterday := today.AddDate(0, 0, -1)
		for i, s := range m.recentSessions {
			if i >= 5 {
				break
			}
			started := s.StartedAt.Local()
			dateStr := started.Format("Jan 02")
			sessionDate := time.Date(started.Year(), started.Month(), started.Day(), 0, 0, 0, 0, started.Location())
			if sessionDate.Equal(today) {
				dateStr = "Today"
			} else if sessionDate.Equal(yesterday) {
				dateStr = "Yest."
			}

			dur := ""
			if s.EndedAt != nil {
				dur = format.Duration(s.Duration())
			}
			bookTitle := s.BookTitle
			if bookTitle == "" {
				bookTitle = "(no book)"
			}
			maxTitle := w - 22
			if maxTitle < 10 {
				maxTitle = 10
			}
			if len(bookTitle) > maxTitle {
				bookTitle = bookTitle[:maxTitle-3] + "..."
			}
			sb.WriteString(fmt.Sprintf("  %-6s  %-*s  %s\n", dateStr, maxTitle, bookTitle, dur))
		}
	}

	// Keybindings hint.
	sb.WriteString("\n")
	if m.timerState != nil {
		sb.WriteString(dimStyleTUI.Render("  [t] or [s] stop"))
	} else {
		sb.WriteString(dimStyleTUI.Render("  [t] start"))
	}

	return sb.String()
}
