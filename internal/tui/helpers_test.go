package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

func runeKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// newTestModel builds a dashboard over an empty fake backend with no
// Hardcover behind it: every remote command reports app.ErrOffline and the
// local store has nothing in it. fakeOf reaches the fake to seed it or take
// it online; nothing is in flight until a test runs Init or a request.
func newTestModel() *Model {
	fb := newFakeBackend()
	fb.online = false
	return newModel(context.Background(), fb, DensityDefault, "test")
}

// fakeOf is the fake behind a test model.
func fakeOf(m *Model) *fakeBackend { return m.backend.(*fakeBackend) }

// showUndoToast puts up a success toast with an undo on offer, the state a
// finished status change or page update leaves behind.
func (m *Model) showUndoToast(text string, undo undoAction) tea.Cmd {
	cmd := m.showToast(toastSuccess, text)
	m.undo = &undo
	return cmd
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

// drainTimeout is how long drain waits on a command it cannot place up
// front. Work against the fake answers at once; a command still running
// after this is a timer, and is left to it.
const drainTimeout = 100 * time.Millisecond

// The commands drain can place without running them: the runner's and the
// requests, which answer at once and are run on the spot, and the two
// timers the dashboard arms, which are skipped without the wait. The timers
// are only a shortcut — a tick that is not recognised is still only a wait
// away from being skipped.
var (
	runFn     = reflect.ValueOf(runCmd{}.run).Pointer()
	tickFn    = reflect.ValueOf(tea.Tick(time.Hour, func(time.Time) tea.Msg { return nil })).Pointer()
	spinnerFn = reflect.ValueOf(spinner.Model{}.Tick).Pointer()
)

// drain runs the work cmd holds the way the runtime would, synchronously:
// every command is executed, a batch taken apart, the runner's results and
// the requests fed back in, any other message delivered, and whatever the
// model answers with drained in turn. A command that does not answer within
// drainTimeout is a timer — the spinner, a toast's expiry, the background
// check, a cursor blink — and is left alone: nothing drain runs may block
// on anything else. The spinner's own tick is dropped rather than fed, since
// feeding it would only arm the next frame. A drained model is at rest with
// nothing in flight.
func drain(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	var msg tea.Msg
	switch reflect.ValueOf(cmd).Pointer() {
	case runFn, requestFn:
		msg = cmd()
	case tickFn, spinnerFn:
		return
	default:
		answer := make(chan tea.Msg, 1)
		go func() { answer <- cmd() }()
		select {
		case msg = <-answer:
		case <-time.After(drainTimeout):
			return
		}
	}
	switch msg := msg.(type) {
	case nil, spinner.TickMsg:
		return
	case tea.BatchMsg:
		for _, c := range msg {
			drain(t, m, c)
		}
	default:
		_, next := m.Update(msg)
		drain(t, m, next)
	}
}

// finish feeds a command's result in the way the runner would, releasing
// the slot of the command a test started, and returns what the root answers.
func finish(t *testing.T, m *Model, msg tea.Msg) tea.Cmd {
	t.Helper()
	return send(t, m, resultMsg{msg})
}

// frameAt is the view's content at a colour profile. lipgloss v2 has no
// global profile: a style always writes its colour and the terminal's writer
// downsamples on the way out, so a test that wants the same bytes wherever it
// runs does that downsampling itself.
func frameAt(m *Model, p colorprofile.Profile) string {
	return atProfile(m.View().Content, p)
}

// atProfile downsamples one rendered string, the way the program's output
// writer would.
func atProfile(s string, p colorprofile.Profile) string {
	var b strings.Builder
	w := &colorprofile.Writer{Forward: &b, Profile: p}
	_, _ = w.WriteString(s)
	return b.String()
}

// layoutProfile is what the layout goldens are written at: no colour and no
// attributes, so a diff shows the frame rather than a wall of escapes.
const layoutProfile = colorprofile.NoTTY

// backgroundMsg is the terminal answering Init's RequestBackgroundColor.
func backgroundMsg(dark bool) tea.BackgroundColorMsg {
	if dark {
		return tea.BackgroundColorMsg{Color: lipgloss.Color("#000000")}
	}
	return tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")}
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
