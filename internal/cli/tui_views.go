package cli

import (
	"fmt"
	"strings"
	"time"
)

const okuASCII = `
              _
   ___   ___ | |_   _  _
  / _ \ | __|| | | | |(_)
 | (_) || (_ | | |_| | _
  \___/  \___|_|\___/ |_|
`

func heatmapBlockTUI(minutes, maxMinutes int) string {
	if minutes <= 0 {
		return heatmapEmptyStyle.Render("░")
	}
	ratio := float64(minutes) / float64(maxMinutes)
	switch {
	case ratio > 0.75:
		return heatmapLevel4Style.Render("█")
	case ratio > 0.5:
		return heatmapLevel3Style.Render("▆")
	case ratio > 0.25:
		return heatmapLevel2Style.Render("▄")
	default:
		return heatmapLevel1Style.Render("░")
	}
}

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

	if m.streakInfo != nil {
		writeField("Streak", fmt.Sprintf("%d days", m.streakInfo.Current))
	}

	if m.timerState != nil {
		elapsed := time.Since(m.timerState.StartedAt)
		bookTitle := ""
		if m.timerState.BookID > 0 && m.app != nil {
			if b, err := m.app.Store.GetBookByID(m.timerState.BookID); err == nil && b != nil {
				bookTitle = b.Title
			}
		}
		if bookTitle != "" {
			writeField("Timer", fmt.Sprintf("%s (%s)", formatDuration(elapsed), bookTitle))
		} else {
			writeField("Timer", formatDuration(elapsed))
		}
	} else {
		writeField("Timer", "not running")
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyleTUI.Render("  j/k navigate   h/l section   ? help"))

	return sb.String()
}

// statsView renders the reading statistics right panel.
func (m dashboardModel) statsView(w int) string {
	var sb strings.Builder

	sb.WriteString(headStyle.Render("Reading Stats"))
	sb.WriteString("\n\n")

	if m.weeklyStats.Sessions == 0 {
		sb.WriteString(dimStyleTUI.Render("  No reading sessions yet.\n"))
		sb.WriteString(dimStyleTUI.Render("  Use 'oku timer start' to begin tracking."))
		return sb.String()
	}

	// Weekly bar chart.
	sb.WriteString(labelStyle.Render("  This Week"))
	sb.WriteString("\n\n")

	dayNames := [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	maxMin := 1
	for _, m := range m.weeklyStats.Days {
		if m > maxMin {
			maxMin = m
		}
	}

	barWidth := w - 16
	if barWidth < 10 {
		barWidth = 10
	}
	if barWidth > 30 {
		barWidth = 30
	}

	for i, mins := range m.weeklyStats.Days {
		filled := 0
		if maxMin > 0 {
			filled = mins * barWidth / maxMin
			if mins > 0 && filled == 0 {
				filled = 1
			}
		}
		empty := barWidth - filled

		bar := statsBarFilledStyle.Render(strings.Repeat("█", filled)) +
			statsBarEmptyStyle.Render(strings.Repeat("░", empty))
		timeStr := "    —"
		if mins > 0 {
			timeStr = fmt.Sprintf("%5s", formatDuration(time.Duration(mins)*time.Minute))
		}
		sb.WriteString(fmt.Sprintf("  %s  %s %s\n", dayNames[i], bar, timeStr))
	}

	sb.WriteString("\n")
	avg := 0
	if m.weeklyStats.Sessions > 0 {
		avg = m.weeklyStats.Total / m.weeklyStats.Sessions
	}
	sb.WriteString(fmt.Sprintf("  Total: %s    Avg: %s    Sessions: %d",
		formatDuration(time.Duration(m.weeklyStats.Total)*time.Minute),
		formatDuration(time.Duration(avg)*time.Minute),
		m.weeklyStats.Sessions,
	))

	return sb.String()
}

// streakView renders the streak + heatmap right panel.
func (m dashboardModel) streakView(w int) string {
	var sb strings.Builder

	sb.WriteString(headStyle.Render("Reading Streaks"))
	sb.WriteString("\n\n")

	if m.streakInfo == nil {
		sb.WriteString(dimStyleTUI.Render("  No reading data yet."))
		return sb.String()
	}

	writeField := func(label, value string) {
		sb.WriteString(labelStyle.Render(fmt.Sprintf("  %-10s ", label)))
		sb.WriteString(valueStyle.Render(value))
		sb.WriteString("\n")
	}

	writeField("Current", fmt.Sprintf("%d days", m.streakInfo.Current))
	writeField("Longest", fmt.Sprintf("%d days", m.streakInfo.Longest))
	writeField("Total", fmt.Sprintf("%d days", m.streakInfo.Total))

	if !m.streakInfo.ReadToday && m.streakInfo.Current > 0 {
		sb.WriteString("\n")
		sb.WriteString(keyStyle.Render("  Read today to keep your streak!"))
	}

	// Heatmap.
	if len(m.heatmapData) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(renderHeatmapTUI(m.heatmapData, 26, w))
	}

	// Weekly bar chart summary.
	if m.weeklyStats.Sessions > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(labelStyle.Render("  This Week"))
		sb.WriteString(valueStyle.Render(fmt.Sprintf("  %s",
			formatDuration(time.Duration(m.weeklyStats.Total)*time.Minute))))
		sb.WriteString(dimStyleTUI.Render(fmt.Sprintf("  |  Avg %s/session",
			formatDuration(time.Duration(m.weeklyStats.Total/max(1, m.weeklyStats.Sessions))*time.Minute))))

		sb.WriteString("\n  ")
		dayNames := [7]string{"M", "T", "W", "T", "F", "S", "S"}
		maxMin := 1
		for _, mins := range m.weeklyStats.Days {
			if mins > maxMin {
				maxMin = mins
			}
		}
		for _, name := range dayNames {
			sb.WriteString(fmt.Sprintf(" %s ", name))
		}
		sb.WriteString("\n  ")
		for _, mins := range m.weeklyStats.Days {
			sb.WriteString(" " + heatmapBlockTUI(mins, maxMin) + " ")
		}
	}

	// Recent sessions.
	if len(m.recentSessions) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(labelStyle.Render("  Recent"))
		sb.WriteString("\n")
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		yesterday := today.AddDate(0, 0, -1)

		for i, s := range m.recentSessions {
			if i >= 4 {
				break
			}
			dateStr := s.StartedAt.Local().Format("Jan 02")
			sessionDate := time.Date(s.StartedAt.Year(), s.StartedAt.Month(), s.StartedAt.Day(), 0, 0, 0, 0, s.StartedAt.Location())
			if sessionDate.Equal(today) {
				dateStr = "Today"
			} else if sessionDate.Equal(yesterday) {
				dateStr = "Yest."
			}

			dur := ""
			if s.EndedAt != nil {
				dur = formatDuration(s.Duration())
			}
			bookTitle := s.BookTitle
			if bookTitle == "" {
				bookTitle = "(no book)"
			}
			// Truncate long titles.
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

	return sb.String()
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
		sb.WriteString(dimStyleTUI.Render("  Press [t] to choose a book and start."))
	} else {
		// Book info.
		if m.timerState.BookID > 0 && m.app != nil {
			if b, err := m.app.Store.GetBookByID(m.timerState.BookID); err == nil && b != nil {
				sb.WriteString(valueStyle.Render("  " + b.Title))
				sb.WriteString("\n")
				if author := b.AuthorString(); author != "" {
					sb.WriteString(dimStyleTUI.Render("  " + author))
					sb.WriteString("\n")
				}
				sb.WriteString("\n")
			}
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
		sessionDate := time.Date(s.StartedAt.Year(), s.StartedAt.Month(), s.StartedAt.Day(), 0, 0, 0, 0, s.StartedAt.Location())
		if sessionDate.Equal(today) {
			todayCount++
			todayMinutes += int(s.Duration().Minutes())
		}
	}
	if todayCount > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("  Today  %d sessions  %s total",
			todayCount, formatDuration(time.Duration(todayMinutes)*time.Minute)))
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
			dateStr := s.StartedAt.Local().Format("Jan 02")
			sessionDate := time.Date(s.StartedAt.Year(), s.StartedAt.Month(), s.StartedAt.Day(), 0, 0, 0, 0, s.StartedAt.Location())
			if sessionDate.Equal(today) {
				dateStr = "Today"
			} else if sessionDate.Equal(yesterday) {
				dateStr = "Yest."
			}

			dur := ""
			if s.EndedAt != nil {
				dur = formatDuration(s.Duration())
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
		sb.WriteString(dimStyleTUI.Render("  [s] stop"))
	} else {
		sb.WriteString(dimStyleTUI.Render("  [t] start"))
	}

	return sb.String()
}
