package tui

import (
	"os"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestDemoTapeKeysStillNavigate replays the recording's key sequences against
// the dashboard, so a keymap change cannot quietly turn the demo into a
// recording of something else. The keys are read out of oku-demo.tape
// itself; the expectations below are what each of them should reach.
//
// The tape starts the dashboard twice, once per palette it shows, and each
// start is replayed against a fresh model.
//
// The one that needs watching is "ll": entering the Search tab must not
// leave a text input holding the keyboard, or the second l would be typed
// into the query instead of moving on to Stats.
func TestDemoTapeKeysStillNavigate(t *testing.T) {
	type step struct {
		key   string
		tab   tab
		focus focus
	}
	want := [][]step{
		{
			{"j", tabReading, focusContent},
			{"j", tabReading, focusContent},
			{"k", tabReading, focusContent},
			{"enter", tabReading, focusDetail},
			{"escape", tabReading, focusContent},
			{"l", tabOku, focusContent},
			{"j", tabOku, focusContent},
			{"l", tabSearch, focusContent},
			{"l", tabStats, focusContent},
			{"l", tabTimer, focusContent},
			{"t", tabTimer, focusContent}, // opens the book picker
			{"j", tabTimer, focusContent},
			{"escape", tabTimer, focusContent}, // cancels it
			{"?", tabTimer, focusContent},      // opens the help modal
			{"escape", tabTimer, focusContent}, // closes it
			{"h", tabStats, focusContent},
			{"h", tabSearch, focusContent},
			{"h", tabOku, focusContent},
			{"h", tabReading, focusContent},
		},
		{
			{"l", tabOku, focusContent},
			{"l", tabSearch, focusContent},
			{"l", tabStats, focusContent},
		},
	}

	sessions := tapeSessions(t)
	if len(sessions) != len(want) {
		t.Fatalf("oku-demo.tape starts the dashboard %d times, the test expects %d", len(sessions), len(want))
	}

	for s, keys := range sessions {
		steps := want[s]
		if len(keys) != len(steps) {
			t.Fatalf("session %d of oku-demo.tape presses %d keys, the test expects %d:\n%v", s+1, len(keys), len(steps), keys)
		}

		m := renderedDashboard(129, 46) // the tape's terminal, near enough
		for i, step := range steps {
			if keys[i] != step.key {
				t.Fatalf("session %d key %d is %q, the test expects %q", s+1, i, keys[i], step.key)
			}
			send(t, m, tapeKeyMsg(t, step.key))

			if m.tab != step.tab {
				t.Fatalf("session %d: after key %d (%q) the tape is on tab %v, want %v", s+1, i, step.key, m.tab, step.tab)
			}
			if m.focus != step.focus {
				t.Fatalf("session %d: after key %d (%q) the focus is %v, want %v", s+1, i, step.key, m.focus, step.focus)
			}
			if got := searchOf(m).input.Value(); got != "" {
				t.Fatalf("session %d: after key %d (%q) the search input reads %q: the tape typed into it", s+1, i, step.key, got)
			}
			if _, help := m.topModal().(*helpModal); help != (step.key == "?") {
				t.Fatalf("session %d: after key %d (%q) the help modal is open: %v", s+1, i, step.key, help)
			}
		}
		if m.topModal() != nil {
			t.Fatalf("session %d: the tape's last Escape should have left no modal open", s+1)
		}
	}
}

// tapeSessions reads the keys oku-demo.tape presses inside the dashboard,
// one slice per `oku` it starts. Everything typed at the shell — the
// `oku config theme` commands and the `oku` that launches — is not a key,
// and the `q` that quits ends a session rather than being one.
func tapeSessions(t *testing.T) [][]string {
	t.Helper()
	raw, err := os.ReadFile("../../oku-demo.tape")
	if err != nil {
		t.Fatalf("reading the demo tape: %v", err)
	}

	var (
		sessions            [][]string
		keys                []string
		launching, launched bool
	)
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case line == "Enter", line == "Escape":
			if launching {
				launching, launched = false, true
				keys = nil
				continue
			}
			if launched {
				keys = append(keys, strings.ToLower(line))
			}
		case strings.HasPrefix(line, "Type "):
			typed, err := strconv.Unquote(strings.TrimSpace(strings.TrimPrefix(line, "Type ")))
			if err != nil {
				t.Fatalf("tape line %q: %v", line, err)
			}
			if !launched {
				launching = typed == "oku"
				continue
			}
			for _, r := range typed {
				if r == 'q' {
					sessions = append(sessions, keys)
					launched = false
					break
				}
				keys = append(keys, string(r))
			}
		}
	}
	if launched {
		t.Fatal("oku-demo.tape leaves the dashboard running: it should end each session with q")
	}
	return sessions
}

func tapeKeyMsg(t *testing.T, name string) tea.KeyPressMsg {
	t.Helper()
	switch name {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "escape":
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	default:
		return runeKey([]rune(name)[0])
	}
}
