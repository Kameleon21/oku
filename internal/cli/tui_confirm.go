package cli

import (
	"strings"

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

func (c *confirmState) handleKey(key string) (confirmed bool, handled bool) {
	switch strings.ToLower(key) {
	case "esc", "n":
		c.Active = false
		return false, true
	case "left", "h", "up", "k":
		c.Cursor = 0
		return false, true
	case "right", "l", "down", "j":
		c.Cursor = 1
		return false, true
	case "y":
		c.Active = false
		return true, true
	case "enter":
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
		Foreground(colorCharcoal).
		Background(colorWarmRed).
		Bold(true).
		Padding(0, 2)
	cancelStyle := lipgloss.NewStyle().
		Foreground(colorCharcoal).
		Background(colorOlive).
		Bold(true).
		Padding(0, 2)
	idleStyle := lipgloss.NewStyle().
		Foreground(colorMidGray).
		Background(colorCharcoal).
		Padding(0, 2)

	left := idleStyle.Render(c.ConfirmText)
	right := idleStyle.Render(c.CancelText)
	if c.Cursor == 0 {
		left = confirmStyle.Render(c.ConfirmText)
	} else {
		right = cancelStyle.Render(c.CancelText)
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
