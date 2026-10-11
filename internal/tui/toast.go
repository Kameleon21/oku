package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/x/ansi"
)

// minStatusMessageWidth is the narrowest a status message may be cut to before
// it is dropped instead: an ellipsis and a couple of letters say nothing.
const minStatusMessageWidth = 8

// undoAction reverses the operation a toast reports. It is data rather than
// a command so the dashboard can say what it will do, and a test can see it.
type undoAction struct {
	op     opKind
	bookID int
	title  string
	// opStatus: the status to go back to, and the one being left.
	toStatus, fromStatus model.Status
	// opProgress: the page to go back to, and the one being left.
	toPage, fromPage int
}

// ── Toasts ─────────────────────────────────────────────────────────────────

// toastLevel is how a toast is drawn: the colour, and the glyph for the
// terminals that have none.
type toastLevel int

const (
	toastInfo    toastLevel = iota // a note: a mode, a cancel, a hint
	toastSuccess                   // something was done
	toastWarn
	toastError
)

// toast is the status bar's message. It expires on its own tick, stamped
// with seq so the tick of a toast that has since been replaced is ignored.
type toast struct {
	level toastLevel
	text  string
	// note follows text and is never cut: a warning that must stay readable
	// however much of the text the footer has room for.
	note string
	seq  int
}

type toastExpiredMsg struct{ seq int }

const (
	toastTTL      = 5 * time.Second
	toastErrorTTL = 8 * time.Second
)

// showToast replaces the status bar's message and returns the tick that
// clears it. The previous toast's undo goes with it: the message that named
// it is gone.
func (m *Model) showToast(level toastLevel, text string) tea.Cmd {
	m.toast = toast{level: level, text: text, seq: m.toast.seq + 1}
	m.undo = nil

	ttl := toastTTL
	if level == toastError {
		ttl = toastErrorTTL
	}
	seq := m.toast.seq
	return tea.Tick(ttl, func(time.Time) tea.Msg {
		return toastExpiredMsg{seq: seq}
	})
}

// toastFor reports an operation's result: its error, or its info with an
// undo when the result says how to reverse it. A warning turns the toast into
// a warning and follows the info, undo and all.
func (m *Model) toastFor(msg opDoneMsg) tea.Cmd {
	if msg.err != nil {
		return m.showToast(toastError, msg.err.Error())
	}
	text := msg.info
	var undo *undoAction
	switch {
	case msg.op == opStatus && msg.bookID > 0 && msg.prevStatus != 0 && msg.prevStatus != msg.newStatus:
		text = fmt.Sprintf("Moved '%s' to %s", msg.title, msg.newStatus.Label())
		undo = &undoAction{op: opStatus, bookID: msg.bookID, title: msg.title,
			toStatus: msg.prevStatus, fromStatus: msg.newStatus}
	case msg.op == opProgress && msg.bookID > 0 && msg.prevPage != msg.newPage:
		text = fmt.Sprintf("Page %d", msg.newPage)
		undo = &undoAction{op: opProgress, bookID: msg.bookID, title: msg.title,
			toPage: msg.prevPage, fromPage: msg.newPage}
	}
	switch {
	case msg.warn == "" && text == "":
		return nil
	case msg.warn == "":
		cmd := m.showToast(toastSuccess, text)
		m.undo = undo
		return cmd
	case text == "":
		return m.showToast(toastWarn, msg.warn)
	}
	cmd := m.showToast(toastWarn, text)
	m.toast.note = msg.warn
	m.undo = undo
	return cmd
}

// runUndo reverses the change the current toast reports, if there is one.
func (m *Model) runUndo() tea.Cmd {
	u := m.undo
	if u == nil {
		return nil
	}
	m.undo = nil
	switch u.op {
	case opStatus:
		return m.runWrite(changeStatusCmd(m.ctx, m.backend, u.bookID, u.title, u.fromStatus, u.toStatus))
	case opProgress:
		return m.runWrite(updateProgressCmd(m.ctx, m.backend, u.bookID, u.title, u.fromPage, strconv.Itoa(u.toPage)))
	}
	return nil
}

// renderToast draws the toast into avail columns of the footer: a glyph for
// the level (so a terminal without colour can tell an error from a note),
// the text cut to what fits, and the undo hint while there is a change to
// undo.
func (m *Model) renderToast(avail int) string {
	if m.toast.text == "" {
		return ""
	}
	style, glyph := m.st.toastInfo, ""
	switch m.toast.level {
	case toastSuccess:
		style, glyph = m.st.toastSuccess, "✓ "
	case toastWarn:
		style, glyph = m.st.toastWarn, "! "
	case toastError:
		style, glyph = m.st.toastError, "✗ "
	}
	undoHint := ""
	if m.undo != nil {
		undoHint = m.st.dim.Render(" · ") +
			m.st.toastAccent.Render("U") +
			m.st.dim.Render(" undo")
	}

	// An API error can carry newlines and runs of whitespace. Left alone they
	// would wrap the bar onto rows the layout has not accounted for, and the
	// help bar would be the thing clipped off the bottom of the screen.
	text := strings.Join(strings.Fields(m.toast.text), " ")

	note := ""
	if m.toast.note != "" {
		note = "; " + m.toast.note
	}

	// A message wider than the footer would wrap it onto a second line and
	// push the layout off the screen, so it is cut to the room that is left.
	// The note is never cut; the undo hint goes first when there is not room
	// for both, and the text after it.
	room := avail - lipgloss.Width(glyph) - lipgloss.Width(note) - lipgloss.Width(undoHint)
	if room < minStatusMessageWidth {
		undoHint = ""
		room = avail - lipgloss.Width(glyph) - lipgloss.Width(note)
	}
	if room < minStatusMessageWidth {
		if m.toast.note != "" && lipgloss.Width(glyph+m.toast.note) <= avail {
			return style.Render(glyph + m.toast.note)
		}
		return ""
	}
	return style.Render(glyph+ansi.Truncate(text, room, "…")+note) + undoHint
}
