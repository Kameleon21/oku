package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// newTestModel builds a dashboard with no app, so every command that would
// touch the network or the store reports an error instead.
func newTestModel() *Model {
	m := New(context.Background(), nil, DensityDefault, "test")
	// Tests drive Update directly and never run Init, so nothing is in flight.
	m.inflight = 0
	return m
}

// The sections behind the interface, for the tests that reach into one.
func readingSection(m *Model) *librarySection { return m.sections[tabReading].(*librarySection) }
func okuSection(m *Model) *librarySection     { return m.sections[tabOku].(*librarySection) }
func searchOf(m *Model) *searchSection        { return m.sections[tabSearch].(*searchSection) }
func statsOf(m *Model) *statsSection          { return m.sections[tabStats].(*statsSection) }

// The modals on the stack, for the tests that assert on one.
func pageModalOf(m *Model) *pageModal {
	p, _ := m.topModal().(*pageModal)
	return p
}

func timerPickerOf(m *Model) *timerPickerModal {
	p, _ := m.topModal().(*timerPickerModal)
	return p
}

// send feeds a message to the model and delivers the request it raises, the
// way the runtime would, returning what is left: the work the root started
// in answer, a bubble's own command, or nil. A tea.Cmd is delivered as it
// is, for a command a section handed back directly.
func send(t *testing.T, m *Model, msg tea.Msg) tea.Cmd {
	t.Helper()
	if cmd, ok := msg.(tea.Cmd); ok {
		return deliver(t, m, cmd)
	}
	_, cmd := m.Update(msg)
	return deliver(t, m, cmd)
}

// requestFn is the one function every request is delivered through, which
// is how a request is told apart from real work without running it.
var requestFn = reflect.ValueOf(request(nil)).Pointer()

// deliver feeds a request back into the model and returns the root's
// answer; any other command is returned unrun.
func deliver(t *testing.T, m *Model, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	if cmd == nil || reflect.ValueOf(cmd).Pointer() != requestFn {
		return cmd
	}
	_, next := m.Update(cmd())
	return next
}

// withColorProfile renders in colour for the duration of a test. Tests have
// no terminal, so lipgloss would otherwise strip every colour and a light and
// a dark render would be the same bytes.
func withColorProfile(t *testing.T) {
	t.Helper()
	setColorProfile(t, termenv.ANSI256, true)
}

// setColorProfile pins the profile and the background for one test, and puts
// both back afterwards. Goldens use it to render the same bytes wherever
// they run.
func setColorProfile(t *testing.T, profile termenv.Profile, dark bool) {
	t.Helper()
	lipgloss.SetColorProfile(profile)
	lipgloss.SetHasDarkBackground(dark)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(termenv.Ascii)
		lipgloss.SetHasDarkBackground(true)
	})
}

// stripANSI removes the escape sequences so a test can look at the glyphs.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
