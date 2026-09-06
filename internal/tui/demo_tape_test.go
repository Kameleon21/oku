package tui

import (
	"os"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestDemoTapeKeysStillNavigate replays the recording's key sequence against
// the dashboard, so a keymap change cannot quietly turn the demo into a
// recording of something else. The keys are read out of oku-demo.tape
// itself; the expectations below are what each of them should reach.
//
// The one that needs watching is "ll": entering the Search tab must not
// leave a text input holding the keyboard, or the second l would be typed
// into the query instead of moving on to Stats.
func TestDemoTapeKeysStillNavigate(t *testing.T) {
	keys := tapeKeys(t)
	want := []struct {
		key   string
		tab   tab
		focus focus
	}{
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
		{"h", tabStats, focusContent},
		{"h", tabSearch, focusContent},
		{"h", tabOku, focusContent},
		{"h", tabReading, focusContent},
	}

	if len(keys) != len(want) {
		t.Fatalf("oku-demo.tape presses %d keys, the test expects %d:\n%v", len(keys), len(want), keys)
	}

	m := renderedDashboard(129, 46) // the tape's terminal, near enough
	for i, step := range want {
		if keys[i] != step.key {
			t.Fatalf("tape key %d is %q, the test expects %q", i, keys[i], step.key)
		}
		send(t, m, tapeKeyMsg(t, step.key))

		if m.tab != step.tab {
			t.Fatalf("after key %d (%q) the tape is on tab %v, want %v", i, step.key, m.tab, step.tab)
		}
		if m.focus != step.focus {
			t.Fatalf("after key %d (%q) the focus is %v, want %v", i, step.key, m.focus, step.focus)
		}
		if got := searchOf(m).input.Value(); got != "" {
			t.Fatalf("after key %d (%q) the search input reads %q: the tape typed into it", i, step.key, got)
		}
	}
	if timerPickerOf(m) != nil {
		t.Fatal("the tape's Escape should have closed the timer picker")
	}
}

// tapeKeys reads the keys oku-demo.tape presses, in order. Only the key
// commands are of interest; Output, Set and Sleep are not.
func tapeKeys(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../../oku-demo.tape")
	if err != nil {
		t.Fatalf("reading the demo tape: %v", err)
	}

	// Everything up to and including the Enter that runs `oku` belongs to
	// the shell, not to the dashboard.
	var keys []string
	launched := false
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case line == "Enter", line == "Escape":
			if !launched {
				launched = true
				continue
			}
			keys = append(keys, strings.ToLower(line))
		case strings.HasPrefix(line, "Type "):
			typed, err := strconv.Unquote(strings.TrimSpace(strings.TrimPrefix(line, "Type ")))
			if err != nil {
				t.Fatalf("tape line %q: %v", line, err)
			}
			if !launched {
				continue
			}
			for _, r := range typed {
				keys = append(keys, string(r))
			}
		}
	}
	return keys
}

func tapeKeyMsg(t *testing.T, name string) tea.KeyMsg {
	t.Helper()
	switch name {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "escape":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return runeKey([]rune(name)[0])
	}
}
