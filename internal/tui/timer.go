package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleTimerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	if m.timerSelecting && m.timerState == nil {
		switch {
		case key.Matches(msg, k.Quit, k.ForceQuit):
			return m, tea.Quit
		case key.Matches(msg, k.Help):
			m.openHelp()
			return m, nil
		case key.Matches(msg, k.Back):
			m.timerSelecting = false
			cmd := m.showToast(toastInfo, "Timer start cancelled")
			return m, cmd
		case key.Matches(msg, k.Down):
			if m.timerSelectIdx < len(m.readingBooks)-1 {
				m.timerSelectIdx++
			}
			return m, nil
		case key.Matches(msg, k.Up):
			if m.timerSelectIdx > 0 {
				m.timerSelectIdx--
			}
			return m, nil
		case key.Matches(msg, k.Select):
			if len(m.readingBooks) == 0 {
				m.timerSelecting = false
				cmd := m.showToast(toastError, "no currently reading books available")
				return m, cmd
			}
			// Background sync can shrink readingBooks while the picker is open.
			if m.timerSelectIdx >= len(m.readingBooks) {
				m.timerSelectIdx = len(m.readingBooks) - 1
			}
			selected := m.readingBooks[m.timerSelectIdx]
			m.timerSelecting = false
			return m.startOp(startTimerForBookCmd(m.app, selected.Book.ID))
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, k.Quit, k.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.openHelp()
		return m, nil
	case key.Matches(msg, k.Down, k.NextSection):
		m.nextSection()
		return m, nil
	case key.Matches(msg, k.Up, k.PrevSection):
		m.prevSection()
		return m, nil
	case key.Matches(msg, k.Search):
		m.focusSearchInput()
		return m, nil
	case key.Matches(msg, k.Timer):
		if m.timerState != nil {
			// Same key, same meaning as in the library: t toggles.
			return m.startOp(stopTimerCmd(m.app))
		}
		if len(m.readingBooks) == 0 {
			cmd := m.showToast(toastError, "no currently reading books available — add a book to Reading first")
			return m, cmd
		}

		m.timerSelecting = true
		m.timerSelectIdx = 0
		if selected := m.selectedLibraryBook(); selected != nil {
			for i, b := range m.readingBooks {
				if b.Book.ID == selected.Book.ID {
					m.timerSelectIdx = i
					break
				}
			}
		}
		cmd := m.showToast(toastInfo, "Select a book and press Enter to start timer")
		return m, cmd
	case key.Matches(msg, k.TimerStop):
		// Only enabled while a timer runs.
		return m.startOp(stopTimerCmd(m.app))
	}
	return m, nil
}

// timerView renders the timer right panel.
func (m Model) timerView(w int) string {
	var sb strings.Builder

	sb.WriteString(m.st.head.Render("Reading Timer"))
	sb.WriteString("\n\n")

	if m.timerSelecting && m.timerState == nil {
		sb.WriteString(m.st.label.Render("  Select a book"))
		sb.WriteString("\n")
		sb.WriteString(m.st.dim.Render("  j/k move   Enter start   Esc cancel"))
		sb.WriteString("\n\n")

		if len(m.readingBooks) == 0 {
			sb.WriteString(m.st.dim.Render("  No books in Reading."))
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
			titleStyle := m.st.value
			if i == m.timerSelectIdx {
				prefix = "▸ "
				titleStyle = m.st.keyHint
			}

			sb.WriteString(titleStyle.Render(prefix + title))
			sb.WriteString("\n")
			sb.WriteString(m.st.dim.Render("  " + author))
			sb.WriteString("\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	if m.timerState == nil {
		sb.WriteString(m.st.dim.Render("  No timer running."))
		sb.WriteString("\n\n")
		sb.WriteString(m.st.dim.Render("  Press [t] to choose a book, or [t] in Reading."))
	} else {
		// Book info.
		if m.timerBook != nil {
			sb.WriteString(m.st.value.Render("  " + m.timerBook.Title))
			sb.WriteString("\n")
			if author := m.timerBook.AuthorString(); author != "" {
				sb.WriteString(m.st.dim.Render("  " + author))
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

		sb.WriteString(m.st.timerDisplay.Render(fmt.Sprintf("       %s", timeStr)))
		sb.WriteString("\n")
		sb.WriteString(m.st.timerLabel.Render("        elapsed"))
		sb.WriteString("\n\n")

		sb.WriteString(m.st.dim.Render(fmt.Sprintf("  Started: %s",
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
		sb.WriteString(m.st.dim.Render("  ────────────────────────────────"))
		sb.WriteString("\n")
		sb.WriteString(m.st.label.Render("  Recent"))
		sb.WriteString("\n")

		for i, s := range m.recentSessions {
			if i >= 5 {
				break
			}
			dateStr := format.DayLabel(s.StartedAt, now)

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
		sb.WriteString(m.st.dim.Render("  [t] or [s] stop"))
	} else {
		sb.WriteString(m.st.dim.Render("  [t] start"))
	}

	return sb.String()
}
