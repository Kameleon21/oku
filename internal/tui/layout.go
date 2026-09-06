package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The rows and columns the frame is built from. Everything else is derived,
// so a change here moves the whole dashboard together.
const (
	headerRows = 1
	footerRows = 1
	// detailMinWidth is the narrowest terminal that still shows the detail
	// pane beside the content pane. Below it Enter swaps one for the other.
	detailMinWidth = 100
	// minContentW keeps a book title readable when the split would otherwise
	// squeeze the list.
	minContentW = 40
	// borderW and padW are what a pane spends on itself: one border column
	// and one space of padding each side.
	borderW, padW = 2, 2
	// minFrameWidth is the narrowest frame worth drawing; below it the rows
	// are padded rather than folded.
	minFrameWidth = 20
)

// layout is the geometry of one frame, computed once per window resize, tab
// switch or detail toggle. Every pane reads its size from here, so the panes
// and the sections inside them can never disagree about the room they have.
type layout struct {
	W, H int
	// BodyH is the rows left between the header and the footer.
	BodyH int
	// Split reports that both panes are drawn side by side.
	Split bool
	// DetailOnly reports that the detail pane has taken the whole width,
	// which is what Enter does on a terminal too narrow to split.
	DetailOnly bool
	// ContentW and DetailW are the outer widths of the two panes, borders
	// included; a pane that is not drawn is zero.
	ContentW, DetailW int
	// PaneH is the outer height of both panes, InnerH the rows inside the
	// border.
	PaneH, InnerH int
	// ContentInner and DetailInner are the columns inside a pane's border
	// and padding: what the section and the detail pane draw into.
	ContentInner, DetailInner int
}

// computeLayout derives the frame's geometry from the terminal size, the tab
// on screen and whether the detail pane has the focus.
func computeLayout(w, h int, t tab, detailFocused bool) layout {
	lay := layout{W: w, H: h}
	if w <= 0 || h <= 0 {
		return lay
	}

	lay.BodyH = max(1, h-headerRows-footerRows)
	lay.PaneH = lay.BodyH
	lay.InnerH = max(1, lay.PaneH-borderW)

	hasDetail := t.hasDetail()
	lay.Split = hasDetail && w >= detailMinWidth
	lay.DetailOnly = hasDetail && !lay.Split && detailFocused

	switch {
	case lay.DetailOnly:
		lay.DetailW = w
	case lay.Split:
		lay.ContentW = max(minContentW, w*2/5)
		lay.DetailW = w - lay.ContentW
	default:
		lay.ContentW = w
	}

	inner := func(outer int) int {
		if outer <= 0 {
			return 0
		}
		return max(1, outer-borderW-padW)
	}
	lay.ContentInner = inner(lay.ContentW)
	lay.DetailInner = inner(lay.DetailW)
	return lay
}

// fitToScreen pads the frame to the terminal and clamps it to those bounds, so
// an unusually long book title or error can never push the layout off-screen
// or wrap it onto a row that does not exist.
func (m *Model) fitToScreen(frame string) string {
	if m.lay.W <= 0 || m.lay.H <= 0 {
		return frame
	}
	return lipgloss.NewStyle().
		MaxWidth(m.lay.W).
		Height(m.lay.H).
		MaxHeight(m.lay.H).
		Render(frame)
}

// ── Panes ──────────────────────────────────────────────────────────────────

// pane draws one bordered box: the title in the top border, body inside it,
// exactly w columns by h rows. The focused pane carries a thick border, so
// which side the keys go to is visible on a terminal without colour.
//
// The box is laid out by hand rather than by a lipgloss border because the
// title sits in the border itself, and because every row has to be exactly w
// wide: a pane that renders one column over would push its neighbour off the
// screen.
func (m *Model) pane(title, body string, w, h int, focused bool) string {
	st := m.st
	border := lipgloss.RoundedBorder()
	borderStyle, titleStyle := st.paneBorder, st.paneTitle
	if focused {
		border = lipgloss.ThickBorder()
		borderStyle, titleStyle = st.paneBorderFocused, st.paneTitleFocused
	}
	if w < 4 || h < 2 {
		return fitBlock(body, max(0, w), max(0, h))
	}

	innerW := w - borderW
	line := func(n int) string {
		if n <= 0 {
			return ""
		}
		return borderStyle.Render(strings.Repeat(border.Top, n))
	}

	// Top border, with the title in it: "┏━ Reading (3) ━━━┓".
	top := borderStyle.Render(border.TopLeft)
	if t := strings.TrimSpace(ansi.Strip(title)); t != "" && innerW >= 6 {
		t = ansi.Truncate(t, innerW-4, "…")
		top += line(1) + " " + titleStyle.Render(t) + " " + line(innerW-3-lipgloss.Width(t))
	} else {
		top += line(innerW)
	}
	top += borderStyle.Render(border.TopRight)

	rows := make([]string, 0, h)
	rows = append(rows, top)
	side := borderStyle.Render(border.Left)
	if inner := fitBlock(body, innerW-padW, h-borderW); inner != "" {
		for _, row := range strings.Split(inner, "\n") {
			rows = append(rows, side+" "+row+" "+side)
		}
	}
	rows = append(rows, borderStyle.Render(border.BottomLeft)+line(innerW)+borderStyle.Render(border.BottomRight))
	return strings.Join(rows, "\n")
}

// fitBlock pins a block of text to exactly w columns by h rows: over-long
// lines are cut instead of wrapped, short ones are padded, and rows past the
// last are dropped. Sections render into it, so what they hand back can
// never stretch the pane around them.
func fitBlock(s string, w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	rows := make([]string, 0, h)
	for i := 0; i < h; i++ {
		row := ""
		if i < len(lines) {
			// ansi.Truncate counts cells, not bytes, so a multi-byte glyph
			// is dropped whole rather than cut into broken runes.
			row = ansi.Truncate(lines[i], w, "")
		}
		if pad := w - lipgloss.Width(row); pad > 0 {
			row += strings.Repeat(" ", pad)
		}
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
}

// stampOverflowBadge right-aligns the badge on the block's last row, in the
// space the pagination dots used to take. The row is overwritten rather than
// appended to: a list pads its rows out to the full width, so there is never
// anything left to append to.
func stampOverflowBadge(content, badge string, w int, st styles) string {
	if badge == "" {
		return content
	}
	badgeW := lipgloss.Width(badge)
	if w <= badgeW+2 {
		return content
	}

	lines := strings.Split(content, "\n")
	last := len(lines) - 1
	head := ansi.Truncate(lines[last], w-badgeW-1, "")
	if pad := w - badgeW - 1 - lipgloss.Width(head); pad > 0 {
		head += strings.Repeat(" ", pad)
	}
	lines[last] = head + st.dim.Render(badge)
	return strings.Join(lines, "\n")
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ── Progress bars ──────────────────────────────────────────────────────────

// progressBar renders a Unicode block-character progress bar with its
// percentage:
//
//	progressBar(45, 300, 20) → "███░░░░░░░░░░░░░░░░░  15%"
func progressBar(current, total, width int, st styles) string {
	if total <= 0 {
		return st.dim.Render(fmt.Sprintf("p.%d", current))
	}
	return fmt.Sprintf("%s %s", bar(current, total, width, st),
		st.dim.Render(fmt.Sprintf("%3d%%", int(percent(current, total)*100))))
}

// bar is the block-character track on its own, for the rows that print the
// numbers themselves.
func bar(current, total, width int, st styles) string {
	if width < 0 {
		width = 0
	}
	filled := int(percent(current, total) * float64(width))
	if filled > width {
		filled = width
	}
	return st.progressFilled.Render(strings.Repeat("█", filled)) +
		st.progressEmpty.Render(strings.Repeat("░", width-filled))
}

// miniProgressBar renders a compact progress bar for inline list items.
func miniProgressBar(current, total, width int) string {
	if total <= 0 {
		return ""
	}
	pct := percent(current, total)
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled) +
		fmt.Sprintf(" %d%%", int(pct*100))
}

// percent is how far through total current is, clamped to 0..1.
func percent(current, total int) float64 {
	if total <= 0 {
		return 0
	}
	pct := float64(current) / float64(total)
	if pct > 1 {
		return 1
	}
	if pct < 0 {
		return 0
	}
	return pct
}
