// Package charts builds the plain-text bar charts and activity heatmap that
// `oku stats` prints and the dashboard draws. Nothing here knows about
// lipgloss: colour arrives as a Palette of string decorators, so the same
// builder produces the CLI's bare glyphs and the TUI's coloured ones.
package charts

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// ── Palette ─────────────────────────────────────────────────────────────────

// Palette decorates a chart. Cell renders one heatmap intensity level (0-4) as
// a single-width glyph; Fill and Empty colour the two runs of a bar; Dim is for
// labels, counts and the legend.
type Palette struct {
	Cell  func(level int) string
	Fill  func(s string) string
	Empty func(s string) string
	Dim   func(s string) string
}

// Plain is the uncoloured palette `oku stats` prints with.
var Plain = Palette{Cell: plainCell, Fill: identity, Empty: identity, Dim: identity}

func identity(s string) string { return s }

// plainCell renders an intensity level as a bare glyph.
func plainCell(level int) string {
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

// fill returns p's decorator for f, or the identity when the palette leaves it
// unset, so a zero Palette still renders readable output.
func (p Palette) fill(s string) string  { return apply(p.Fill, s) }
func (p Palette) empty(s string) string { return apply(p.Empty, s) }
func (p Palette) dim(s string) string   { return apply(p.Dim, s) }

func (p Palette) cell(level int) string {
	if p.Cell == nil {
		return plainCell(level)
	}
	return p.Cell(level)
}

func apply(f func(string) string, s string) string {
	if f == nil {
		return s
	}
	return f(s)
}

// ── Horizontal bar chart ────────────────────────────────────────────────────

// BarChartH renders labeled horizontal bars scaled to the largest count:
//
//	Jan ██████░░░░ 3
//
// Rows with a zero count render an empty track. labelW pads labels for
// alignment; barW is the bar track width.
func BarChartH(rows []model.LabelCount, labelW, barW int, p Palette) string {
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
		bar := p.fill(strings.Repeat("█", f)) +
			p.empty(strings.Repeat("░", barW-f))
		countStr := " "
		if r.Count > 0 {
			countStr = fmt.Sprintf("%d", r.Count)
		}
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("  %-*s %s %s", labelW, r.Label, bar, p.dim(countStr)))
	}
	return sb.String()
}

// ── Activity heatmap (GitHub-style) ─────────────────────────────────────────

// HeatmapPrefixW is the width of the row-label gutter ("  Mo  "); each week
// column is HeatmapWeekW chars wide (cell + space).
const (
	HeatmapPrefixW = 6
	HeatmapWeekW   = 2
)

// HeatmapLevel maps a day's activity to an intensity level 0 (none) to 4
// (highest). Timer minutes scale relative to the busiest day; journal entries
// use absolute buckets so a day with one progress update stays light and a
// heavy logging day reads dark. The stronger of the two wins.
func HeatmapLevel(a model.DayActivity, maxMinutes int) int {
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

// FitHeatmapWeeks caps weeks at what availWidth can draw (gutter plus two
// columns per week), never going below four weeks: a stub of a grid still says
// more than none.
func FitHeatmapWeeks(weeks, availWidth int) int {
	maxWeeks := (availWidth - HeatmapPrefixW - 2) / HeatmapWeekW
	if maxWeeks < 4 {
		maxWeeks = 4
	}
	if weeks > maxWeeks {
		weeks = maxWeeks
	}
	return weeks
}

// Heatmap renders a GitHub-style activity grid ending at today: month labels
// aligned to their week columns, all seven weekday rows, and a Less→More
// legend.
func Heatmap(activities []model.DayActivity, weeks int, p Palette) string {
	return HeatmapAt(activities, weeks, time.Now(), p)
}

// HeatmapAt is Heatmap with an explicit "today", so the grid and its month
// labels can be tested against fixed dates.
func HeatmapAt(activities []model.DayActivity, weeks int, now time.Time, p Palette) string {
	if weeks < 1 {
		weeks = 1
	}

	actMap := make(map[string]model.DayActivity, len(activities))
	maxMin := 1
	for _, a := range activities {
		actMap[a.Date.Format("2006-01-02")] = a
		if a.Minutes > maxMin {
			maxMin = a.Minutes
		}
	}

	weekday := (int(now.Weekday()) + 6) % 7 // Mon=0 .. Sun=6
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startDate := endDate.AddDate(0, 0, -(weeks-1)*7-weekday)

	var sb strings.Builder

	// Month labels, written into a buffer so each lands on the column of the
	// week that contains the 1st of that month. A column's month is taken from
	// its Sunday: it changes exactly when a month boundary falls somewhere in
	// the column's Mon–Sun span, so a month starting mid-week is still labelled.
	labels := make([]rune, HeatmapPrefixW+weeks*HeatmapWeekW+len("Jan"))
	for i := range labels {
		labels[i] = ' '
	}
	lastMonth := time.Month(0)
	lastPos, lastEnd := -1, -1
	for w := 0; w < weeks; w++ {
		sunday := startDate.AddDate(0, 0, w*7+6)
		m := sunday.Month()
		if m == lastMonth {
			continue
		}
		lastMonth = m
		pos := HeatmapPrefixW + w*HeatmapWeekW
		if pos <= lastEnd {
			// Labels are wider than a week column, so two can only collide
			// when the grid opens on a stub of a month that ends within the
			// next column. Prefer the real month boundary over the stub.
			if lastPos != HeatmapPrefixW {
				continue
			}
			for i := lastPos; i < lastEnd; i++ {
				labels[i] = ' '
			}
		}
		name := sunday.Format("Jan")
		copy(labels[pos:], []rune(name))
		lastPos, lastEnd = pos, pos+len(name)
	}
	sb.WriteString(p.dim(strings.TrimRight(string(labels), " ")))
	sb.WriteString("\n")

	dayLabels := [7]string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	for dayIdx := 0; dayIdx < 7; dayIdx++ {
		sb.WriteString(p.dim(fmt.Sprintf("  %s  ", dayLabels[dayIdx])))
		for w := 0; w < weeks; w++ {
			d := startDate.AddDate(0, 0, w*7+dayIdx)
			if d.After(endDate) {
				sb.WriteString("  ")
				continue
			}
			act := actMap[d.Format("2006-01-02")]
			sb.WriteString(p.cell(HeatmapLevel(act, maxMin)) + " ")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n" + p.dim("  Less "))
	for lvl := 0; lvl <= 4; lvl++ {
		sb.WriteString(p.cell(lvl) + " ")
	}
	sb.WriteString(p.dim("More"))

	return sb.String()
}
