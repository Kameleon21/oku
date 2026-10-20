package tui

import (
	"os"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/config"
)

// TestDemoTapeKeysStillNavigate replays the actual tape against the model,
// including live theme previews and saves, so the recording cannot silently
// type theme names into Search or leave a picker open.
func TestDemoTapeKeysStillNavigate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	old := currentThemeSetting()
	t.Cleanup(func() { _ = ApplyThemeSetting(old) })
	_ = ApplyThemeSetting("oku")
	sessions := tapeSessions(t)
	if len(sessions) != 1 {
		t.Fatalf("want one TUI session, got %d", len(sessions))
	}
	m := renderedDashboard(115, 36)
	steps := []struct {
		keys   string
		tab    tab
		focus  focus
		theme  string
		picker bool
	}{
		{"j", tabReading, focusContent, "oku", false},
		{"4", tabStats, focusContent, "oku", false},
		{"T", tabStats, focusContent, "oku", true},
		{"oku-light", tabStats, focusContent, "oku-light", true},
		{"enter", tabStats, focusContent, "oku-light", false},
		{"2", tabOku, focusContent, "oku-light", false},
		{"T", tabOku, focusContent, "oku-light", true},
		{"nord", tabOku, focusContent, "nord", true},
		{"enter", tabOku, focusContent, "nord", false},
		{"1", tabReading, focusContent, "nord", false},
		{"T", tabReading, focusContent, "nord", true},
		{"oku", tabReading, focusContent, "oku", true},
		{"enter", tabReading, focusContent, "oku", false},
		{"k", tabReading, focusContent, "oku", false},
	}
	i := 0
	for _, step := range steps {
		keys := []string{step.keys}
		if step.keys != "enter" && step.keys != "escape" {
			keys = strings.Split(step.keys, "")
		}
		for _, key := range keys {
			if i >= len(sessions[0]) || sessions[0][i] != key {
				t.Fatalf("tape key %d does not match expected %q", i, key)
			}
			send(t, m, tapeKeyMsg(t, key))
			i++
		}
		if m.tab != step.tab || m.focus != step.focus || currentThemeSetting() != step.theme {
			t.Fatalf("after %q: tab=%v focus=%v theme=%s", step.keys, m.tab, m.focus, currentThemeSetting())
		}
		_, picker := m.topModal().(*themePicker)
		if picker != step.picker {
			t.Fatalf("after %q: theme picker open=%v, want %v", step.keys, picker, step.picker)
		}
		if searchOf(m).input.Value() != "" {
			t.Fatal("the tape typed into Search")
		}
		if step.keys == "enter" && !step.picker {
			cfg, err := config.Load()
			if err != nil || cfg.Theme != step.theme {
				t.Fatalf("theme was not saved: config=%+v err=%v", cfg, err)
			}
		}
	}
	if i != len(sessions[0]) {
		t.Fatalf("tape has %d unverified keys", len(sessions[0])-i)
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
