package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// timerSection shows the running timer, or how to start one, with today's
// total and the recent sessions. Picking the book happens in the
// timerPickerModal.
type timerSection struct {
	sh *shared
	st styles
}

func newTimerSection(sh *shared, st styles) *timerSection {
	return &timerSection{sh: sh, st: st}
}

func (s *timerSection) Update(msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	k := keysFor(s)
	switch {
	case key.Matches(keyMsg, k.Down):
		return request(reqSwitchTab{step: +1})
	case key.Matches(keyMsg, k.Up):
		return request(reqSwitchTab{step: -1})
	case key.Matches(keyMsg, k.Timer):
		if s.sh.timer != nil {
			// Same key, same meaning as in the library: t toggles.
			return request(reqTimer{})
		}
		return request(reqTimerPick{})
	case key.Matches(keyMsg, k.TimerStop):
		// Only enabled while a timer runs.
		return request(reqTimer{})
	}
	return nil
}

// View renders the timer page.
func (s *timerSection) View(w, _ int) string {
	st := s.st
	var sb strings.Builder

	sb.WriteString(st.head.Render("Reading Timer"))
	sb.WriteString("\n\n")

	now := s.sh.now()
	if s.sh.timer == nil {
		sb.WriteString(st.dim.Render("  No timer running."))
		sb.WriteString("\n\n")
		sb.WriteString(st.dim.Render("  Press [t] to choose a book, or [t] in Reading."))
	} else {
		// Book info.
		if s.sh.timerBook != nil {
			sb.WriteString(st.value.Render("  " + s.sh.timerBook.Title))
			sb.WriteString("\n")
			if author := s.sh.timerBook.AuthorString(); author != "" {
				sb.WriteString(st.dim.Render("  " + author))
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}

		// Large timer display.
		elapsed := now.Sub(s.sh.timer.StartedAt)
		h := int(elapsed.Hours())
		min := int(elapsed.Minutes()) % 60
		sec := int(elapsed.Seconds()) % 60
		timeStr := fmt.Sprintf("%02d:%02d:%02d", h, min, sec)

		sb.WriteString(st.timerDisplay.Render(fmt.Sprintf("       %s", timeStr)))
		sb.WriteString("\n")
		sb.WriteString(st.timerLabel.Render("        elapsed"))
		sb.WriteString("\n\n")

		sb.WriteString(st.dim.Render(fmt.Sprintf("  Started: %s",
			s.sh.timer.StartedAt.Local().Format("3:04 PM"))))
	}

	// Today's stats.
	todayCount := 0
	todayMinutes := 0
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for _, sess := range s.sh.sessions {
		started := sess.StartedAt.Local()
		sessionDate := time.Date(started.Year(), started.Month(), started.Day(), 0, 0, 0, 0, started.Location())
		if sessionDate.Equal(today) {
			todayCount++
			todayMinutes += int(sess.Duration().Minutes())
		}
	}
	if todayCount > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("  Today  %d sessions  %s total",
			todayCount, format.Duration(time.Duration(todayMinutes)*time.Minute)))
	}

	// Recent sessions.
	if len(s.sh.sessions) > 0 {
		sb.WriteString("\n")
		sb.WriteString(st.dim.Render("  ────────────────────────────────"))
		sb.WriteString("\n")
		sb.WriteString(st.label.Render("  Recent"))
		sb.WriteString("\n")

		for i, sess := range s.sh.sessions {
			if i >= 5 {
				break
			}
			dateStr := format.DayLabel(sess.StartedAt, now)

			dur := ""
			if sess.EndedAt != nil {
				dur = format.Duration(sess.Duration())
			}
			bookTitle := sess.BookTitle
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
	if s.sh.timer != nil {
		sb.WriteString(st.dim.Render("  [t] or [s] stop"))
	} else {
		sb.WriteString(st.dim.Render("  [t] start"))
	}

	return sb.String()
}

func (s *timerSection) Resize(int, int) {}

func (s *timerSection) Keys(k *keyMap) {
	sectionHint := hint("section", k.PrevSection, k.NextSection)
	k.Up.SetHelp("k", "section")
	k.Down.SetHelp("j", "section")
	if s.sh.timer != nil {
		k.Timer.SetHelp("t", "stop timer")
		enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.NextSection, &k.PrevSection, &k.Search,
			&k.Timer, &k.TimerStop)
		k.short = []key.Binding{k.Help, hint("stop", k.Timer, k.TimerStop), sectionHint, k.Search, k.Quit}
		return
	}
	k.Timer.SetHelp("t", "choose + start")
	enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.NextSection, &k.PrevSection, &k.Search, &k.Timer)
	k.short = []key.Binding{k.Help, k.Timer, sectionHint, k.Search, k.Quit}
}

func (s *timerSection) Focus() {}
func (s *timerSection) Blur()  {}

func (s *timerSection) CapturesKeys() bool { return false }

func (s *timerSection) Title() string { return "Timer" }

func (s *timerSection) Selected() selection { return selection{} }
