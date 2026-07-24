package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/lipgloss"
)

// ── Stat line ───────────────────────────────────────────────────────────────

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

// groupThousands formats an int with thousands separators: 1748 → "1,748".
func groupThousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var sb strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		sb.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if sb.Len() > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(s[i : i+3])
	}
	return sb.String()
}

// ── Horizontal bar chart ────────────────────────────────────────────────────

// renderBarChartH renders labeled horizontal bars scaled to the largest count:
//
//	Jan ██████░░░░ 3
//
// Rows with a zero count render an empty track. labelW pads labels for
// alignment; barW is the bar track width.
func renderBarChartH(rows []model.LabelCount, labelW, barW int, filled lipgloss.Style) string {
	if len(rows) == 0 {
		return ""
	}
	maxCount := 1
	for _, r := range rows {
		if r.Count > maxCount {
			maxCount = r.Count
		}
	}

	var sb strings.Builder
	for i, r := range rows {
		f := r.Count * barW / maxCount
		if r.Count > 0 && f == 0 {
			f = 1
		}
		bar := filled.Render(strings.Repeat("█", f)) +
			statsBarEmptyStyle.Render(strings.Repeat("░", barW-f))
		countStr := " "
		if r.Count > 0 {
			countStr = fmt.Sprintf("%d", r.Count)
		}
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("  %-*s %s %s", labelW, r.Label, bar, dimStyleTUI.Render(countStr)))
	}
	return sb.String()
}

// ── Activity heatmap (GitHub-style) ─────────────────────────────────────────

// heatmapPrefixW is the width of the row-label gutter ("  Mo  "); each week
// column is heatmapWeekW chars wide (cell + space).
const (
	heatmapPrefixW = 6
	heatmapWeekW   = 2
)

// heatmapLevel maps a day's activity to an intensity level 0 (none) to 4
// (highest). Timer minutes scale relative to the busiest day; journal entries
// use absolute buckets so a day with one progress update stays light and a
// heavy logging day reads dark. The stronger of the two wins.
func heatmapLevel(a model.DayActivity, maxMinutes int) int {
	level := 0
	if a.Minutes > 0 && maxMinutes > 0 {
		ratio := float64(a.Minutes) / float64(maxMinutes)
		switch {
		case ratio > 0.75:
			level = 4
		case ratio > 0.5:
			level = 3
		case ratio > 0.25:
			level = 2
		default:
			level = 1
		}
	}
	entryLevel := 0
	switch {
	case a.Entries >= 6:
		entryLevel = 4
	case a.Entries >= 4:
		entryLevel = 3
	case a.Entries >= 2:
		entryLevel = 2
	case a.Entries >= 1 || a.HasActivity:
		entryLevel = 1
	}
	if entryLevel > level {
		level = entryLevel
	}
	return level
}

// heatmapCellPlain renders an intensity level as a bare glyph (for `oku stats`).
func heatmapCellPlain(level int) string {
	switch level {
	case 4:
		return "█"
	case 3:
		return "▓"
	case 2:
		return "▒"
	case 1:
		return "░"
	default:
		return "·"
	}
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

// buildHeatmap renders a GitHub-style activity grid ending at today: month
// labels aligned to their week columns, all seven weekday rows, and a
// Less→More legend. cell renders one intensity level (0-4) as a single-width
// glyph; dim styles the labels and may be nil for plain output.
func buildHeatmap(activities []model.DayActivity, weeks int, cell func(level int) string, dim func(string) string) string {
	if dim == nil {
		dim = func(s string) string { return s }
	}

	actMap := make(map[string]model.DayActivity, len(activities))
	maxMin := 1
	for _, a := range activities {
		actMap[a.Date.Format("2006-01-02")] = a
		if a.Minutes > maxMin {
			maxMin = a.Minutes
		}
	}

	now := time.Now()
	weekday := (int(now.Weekday()) + 6) % 7 // Mon=0 .. Sun=6
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startDate := endDate.AddDate(0, 0, -(weeks-1)*7-weekday)

	var sb strings.Builder

	// Month labels, written into a buffer so each lands on the column of the
	// week where that month starts.
	labels := make([]rune, heatmapPrefixW+weeks*heatmapWeekW+len("Jan"))
	for i := range labels {
		labels[i] = ' '
	}
	lastMonth := -1
	lastEnd := -1
	for w := 0; w < weeks; w++ {
		d := startDate.AddDate(0, 0, w*7)
		m := int(d.Month())
		if m == lastMonth {
			continue
		}
		lastMonth = m
		pos := heatmapPrefixW + w*heatmapWeekW
		if pos <= lastEnd {
			continue
		}
		name := d.Format("Jan")
		copy(labels[pos:], []rune(name))
		lastEnd = pos + len(name)
	}
	sb.WriteString(dim(strings.TrimRight(string(labels), " ")))
	sb.WriteString("\n")

	dayLabels := [7]string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	for dayIdx := 0; dayIdx < 7; dayIdx++ {
		sb.WriteString(dim(fmt.Sprintf("  %s  ", dayLabels[dayIdx])))
		for w := 0; w < weeks; w++ {
			d := startDate.AddDate(0, 0, w*7+dayIdx)
			if d.After(endDate) {
				sb.WriteString("  ")
				continue
			}
			act := actMap[d.Format("2006-01-02")]
			sb.WriteString(cell(heatmapLevel(act, maxMin)) + " ")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n" + dim("  Less "))
	for lvl := 0; lvl <= 4; lvl++ {
		sb.WriteString(cell(lvl) + " ")
	}
	sb.WriteString(dim("More"))

	return sb.String()
}

// renderHeatmapTUI returns a heatmap string for the TUI (no printing to stdout).
func renderHeatmapTUI(activities []model.DayActivity, weeks, availWidth int) string {
	// Adapt weeks to available width: gutter + 2 chars per week.
	maxWeeks := (availWidth - heatmapPrefixW - 2) / heatmapWeekW
	if maxWeeks < 4 {
		maxWeeks = 4
	}
	if weeks > maxWeeks {
		weeks = maxWeeks
	}
	return buildHeatmap(activities, weeks, heatmapCellStyled, func(s string) string { return dimStyleTUI.Render(s) })
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
