package cli

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmState is a reusable, keyboard-driven modal confirmation state.
type confirmState struct {
	Active      bool
	Message     string
	ConfirmText string
	CancelText  string
	Cursor      int // 0=confirm, 1=cancel
}

func newConfirmState(message string) confirmState {
	return confirmState{
		Active:      true,
		Message:     message,
		ConfirmText: "Confirm",
		CancelText:  "Cancel",
		Cursor:      1,
	}
}

// handleKey answers the question with the confirm bindings of k: yes and no
// close it, the arrows move between the buttons, and Select takes the one
// under the cursor.
func (c *confirmState) handleKey(msg tea.KeyMsg, k keyMap) (confirmed bool, handled bool) {
	switch {
	case key.Matches(msg, k.ConfirmNo):
		c.Active = false
		return false, true
	case key.Matches(msg, k.ConfirmLeft):
		c.Cursor = 0
		return false, true
	case key.Matches(msg, k.ConfirmRight):
		c.Cursor = 1
		return false, true
	case key.Matches(msg, k.ConfirmYes):
		c.Active = false
		return true, true
	case key.Matches(msg, k.Select):
		c.Active = false
		return c.Cursor == 0, true
	}
	return false, false
}

func renderConfirmModal(c confirmState, width int) string {
	if width <= 0 {
		width = 50
	}
	if width < 36 {
		width = 36
	}

	confirmStyle := lipgloss.NewStyle().
		Foreground(th.surface).
		Background(th.error).
		Bold(true).
		Padding(0, 2)
	cancelStyle := lipgloss.NewStyle().
		Foreground(th.surface).
		Background(th.success).
		Bold(true).
		Padding(0, 2)
	idleStyle := lipgloss.NewStyle().
		Foreground(th.textMuted).
		Background(th.surface).
		Padding(0, 2)

	// The chosen button is marked as well as coloured, so the choice is
	// visible on a terminal without colour.
	left := idleStyle.Render("  " + c.ConfirmText)
	right := idleStyle.Render("  " + c.CancelText)
	if c.Cursor == 0 {
		left = confirmStyle.Render("▸ " + c.ConfirmText)
	} else {
		right = cancelStyle.Render("▸ " + c.CancelText)
	}

	// Every part of the row carries the modal background: the gap between the
	// buttons and the space either side of them included, or a black band
	// shows through the charcoal panel.
	buttons := modalBgStyle.
		Width(width - 6).
		Align(lipgloss.Center).
		Render(lipgloss.JoinHorizontal(lipgloss.Center, left, modalBgStyle.Render("  "), right))

	content := modalValueStyle.Render(c.Message) + "\n\n" +
		buttons + "\n\n" +
		modalDimStyle.Render("y/n or Enter/Esc")
	return renderModalPanel("Confirm", content, width)
}
