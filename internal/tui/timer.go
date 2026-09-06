package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/format"
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
	if st, ok := msg.(stylesChangedMsg); ok {
		s.st = st.st
		return nil
	}
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	k := keysFor(s)
	switch {
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
func (s *timerSection) View(w, h int) string {
	return fitBlock(s.page(w), w, h)
}

func (s *timerSection) page(w int) string {
	// No heading: the pane's own title already names the page.
	st := s.st
	var sb strings.Builder

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

func (s *timerSection) Resize(int, int) tea.Cmd { return nil }

// Keys leaves j/k out: the tab strip moves between pages now, and this one
// is a single screen with nothing to scroll.
func (s *timerSection) Keys(k *keyMap) {
	tabHint := hint("tab", k.PrevSection, k.NextSection)
	if s.sh.timer != nil {
		k.Timer.SetHelp("t", "stop timer")
		enable(&k.Quit, &k.Help, &k.NextSection, &k.PrevSection, &k.TabJump, &k.Search,
			&k.Timer, &k.TimerStop)
		k.short = []key.Binding{k.Help, hint("stop", k.Timer, k.TimerStop), tabHint, k.Search, k.Quit}
		return
	}
	k.Timer.SetHelp("t", "choose + start")
	enable(&k.Quit, &k.Help, &k.NextSection, &k.PrevSection, &k.TabJump, &k.Search, &k.Timer)
	k.short = []key.Binding{k.Help, k.Timer, tabHint, k.Search, k.Quit}
}

func (s *timerSection) Focus() {}
func (s *timerSection) Blur()  {}

func (s *timerSection) CapturesKeys() bool { return false }

func (s *timerSection) Title() string { return "Timer" }

func (s *timerSection) Selected() selection { return selection{} }
