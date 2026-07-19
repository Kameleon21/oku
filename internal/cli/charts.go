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

func intensityCellTUI(minutes, maxMinutes int, active bool) string {
	if minutes <= 0 {
		if active {
			return heatmapLevel2Style.Render("▪")
		}
		return heatmapEmptyStyle.Render("·")
	}
	ratio := float64(minutes) / float64(maxMinutes)
	switch {
	case ratio > 0.75:
		return heatmapLevel4Style.Render("█")
	case ratio > 0.5:
		return heatmapLevel3Style.Render("▓")
	case ratio > 0.25:
		return heatmapLevel2Style.Render("▒")
	default:
		return heatmapLevel1Style.Render("░")
	}
}

// renderHeatmapTUI returns a heatmap string for the TUI (no printing to stdout).
func renderHeatmapTUI(activities []model.DayActivity, weeks, availWidth int) string {
	actMap := make(map[string]model.DayActivity, len(activities))
	for _, a := range activities {
		actMap[a.Date.Format("2006-01-02")] = a
	}

	maxMin := 1
	for _, a := range activities {
		if a.Minutes > maxMin {
			maxMin = a.Minutes
		}
	}

	// Adapt weeks to available width: each week takes ~3 chars.
	maxWeeks := (availWidth - 8) / 3
	if maxWeeks < 4 {
		maxWeeks = 4
	}
	if weeks > maxWeeks {
		weeks = maxWeeks
	}

	now := time.Now()
	weekday := (int(now.Weekday()) + 6) % 7
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startDate := endDate.AddDate(0, 0, -(weeks-1)*7-weekday)

	var sb strings.Builder

	// Month labels.
	sb.WriteString("       ")
	lastMonth := -1
	for w := 0; w < weeks; w++ {
		d := startDate.AddDate(0, 0, w*7)
		m := int(d.Month())
		if m != lastMonth {
			sb.WriteString(fmt.Sprintf("%-3s", d.Format("Jan")))
			lastMonth = m
		} else {
			sb.WriteString("   ")
		}
	}
	sb.WriteString("\n")

	// Rows.
	dayLabels := [7]string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	displayRows := []int{0, 2, 4, 6}
	for _, dayIdx := range displayRows {
		sb.WriteString(fmt.Sprintf("  %s   ", dayLabels[dayIdx]))
		for w := 0; w < weeks; w++ {
			d := startDate.AddDate(0, 0, w*7+dayIdx)
			if d.After(endDate) {
				sb.WriteString("  ")
				continue
			}
			key := d.Format("2006-01-02")
			act := actMap[key]
			sb.WriteString(intensityCellTUI(act.Minutes, maxMin, act.HasActivity) + " ")
		}
		sb.WriteString("\n")
	}

	// Legend.
	sb.WriteString("\n  Less ")
	sb.WriteString(intensityCellTUI(0, maxMin, false))
	sb.WriteString(" ")
	sb.WriteString(intensityCellTUI(maxMin/4, maxMin, false))
	sb.WriteString(" ")
	sb.WriteString(intensityCellTUI(maxMin/2, maxMin, false))
	sb.WriteString(" ")
	sb.WriteString(intensityCellTUI(maxMin, maxMin, false))
	sb.WriteString(" More")

	return sb.String()
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
