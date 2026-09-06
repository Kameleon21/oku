package tui

import (
	"fmt"
	"strings"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// layout is the geometry the frame is drawn into: the terminal size. The
// six-card layout derives its panes from it as it draws.
type layout struct {
	W, H int
}

// rightPanelContentWidth mirrors renderLayout's width math for the right
// panel's content area.
func (l layout) rightPanelContentWidth() int {
	totalW := max(60, l.W-2)
	leftW := max(28, totalW*2/5)
	rightW := max(28, l.W-leftW-4)
	return rightW - 4
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

// chromeRows counts the rows View draws outside the two-column layout: the
// status bar, plus whatever footer the current mode prints under it.
func (m *Model) chromeRows() int {
	if m.pagePrompt() != nil {
		return 1 + pagePromptRows
	}
	return 1 + 1 // status bar + help bar
}

// layoutHeight is the height of the two-column layout, borders included. It
// takes every row the chrome does not, so the panels reach the bottom of the
// terminal instead of leaving it blank.
func (m *Model) layoutHeight() int {
	return max(8, m.lay.H-m.chromeRows())
}

// rightPanelContentHeight mirrors renderLayout's height math for the right
// panel's content area.
func (m *Model) rightPanelContentHeight() int {
	return max(1, m.layoutHeight()-2)
}

// renderLayout renders the 2-column layout: left sections + right context panel.
func (m *Model) renderLayout() string {
	totalW := max(60, m.lay.W-2)
	panelInnerH := m.rightPanelContentHeight()
	leftW := max(28, totalW*2/5)

	// The left frame is only a frame: the focus cue belongs to the card
	// inside it, or to the right pane when the cursor is over there.
	leftContent := clampPanelContent(m.renderSections(leftW-2, panelInnerH), leftW, panelInnerH)
	leftPanel := m.st.pane.Width(leftW).Height(panelInnerH).Render(leftContent)

	// Right panel: context-sensitive.
	rightW := max(28, m.lay.W-lipgloss.Width(leftPanel)-2)
	rightContent := clampPanelContent(m.rightPanelView(rightW-4, panelInnerH), rightW, panelInnerH)
	rightStyle := m.st.pane
	if m.rightPaneFocused() {
		rightStyle = m.st.paneFocused
	}
	rightPanel := rightStyle.
		Width(rightW).
		Height(panelInnerH).
		Render(rightContent)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
}

// rightPaneFocused reports whether j/k act on the right pane: over the search
// results, the timer's book picker or the stats page. The pane then carries
// the focus border, and the section card keeps its marker.
func (m *Model) rightPaneFocused() bool {
	switch m.tab {
	case sectionSearch:
		return m.search.inResults()
	case sectionTimer:
		return m.timerPicker() != nil
	case sectionStats:
		return true
	}
	return false
}

// clampPanelContent pins a panel's content to its box: over-long lines are cut
// instead of wrapped, and content past the last row is dropped, so one long
// title can never stretch the layout past the bottom of the terminal.
func clampPanelContent(content string, w, h int) string {
	return lipgloss.NewStyle().
		MaxWidth(w).
		Height(h).
		MaxHeight(h).
		Render(content)
}

// renderSections renders the left panel content: section labels + expanded section.
func (m *Model) renderSections(w, h int) string {
	defs := m.sectionDefinitions()
	if len(defs) == 0 {
		return ""
	}

	heights := m.leftSectionHeights(h)
	parts := make([]string, 0, len(defs))
	for _, def := range defs {
		// A zero-height card still costs a row once joined, so drop it.
		if heights[def.id] <= 0 {
			continue
		}
		parts = append(parts, m.renderSectionCard(def, w, heights[def.id], def.id == m.tab))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *Model) sectionDefinitions() []sectionDef {
	return []sectionDef{
		{sectionIntro, "Intro", -1},
		{sectionReading, "Reading", len(m.shared.reading)},
		{sectionOku, "Oku", len(m.shared.oku)},
		{sectionSearch, "Search Titles", -1},
		{sectionStats, "Stats", -1},
		{sectionTimer, "Timer", -1},
	}
}

func (m *Model) leftSectionHeights(totalH int) map[focusSection]int {
	heights := map[focusSection]int{
		sectionIntro:  3,
		sectionSearch: 4,
		sectionStats:  3,
		sectionTimer:  3,
	}
	minHeights := map[focusSection]int{
		sectionIntro:  2,
		sectionSearch: 3,
		sectionStats:  2,
		sectionTimer:  2,
	}
	// Intro gives up its box first: it is the one card whose whole content is
	// its label, so it is the one that loses nothing by being drawn bare.
	reduceOrder := []focusSection{
		sectionIntro, sectionStats, sectionTimer, sectionSearch,
	}

	fixedSum := heights[sectionIntro] + heights[sectionSearch] + heights[sectionStats] + heights[sectionTimer]
	remaining := totalH - fixedSum
	for remaining < 8 {
		changed := false
		for _, id := range reduceOrder {
			if remaining >= 8 {
				break
			}
			if heights[id] > minHeights[id] {
				heights[id]--
				fixedSum--
				remaining++
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	readingH := max(4, remaining*3/5)
	okuH := max(4, remaining-readingH)
	for readingH+okuH > remaining && (readingH > 2 || okuH > 2) {
		if readingH >= okuH && readingH > 2 {
			readingH--
		} else if okuH > 2 {
			okuH--
		}
	}
	for readingH+okuH < remaining {
		readingH++
	}

	if m.tab == sectionReading && okuH > 4 {
		shift := min(2, okuH-4)
		okuH -= shift
		readingH += shift
	}
	if m.tab == sectionOku && readingH > 4 {
		shift := min(2, readingH-4)
		readingH -= shift
		okuH += shift
	}

	heights[sectionReading] = readingH
	heights[sectionOku] = okuH

	sum := 0
	for _, def := range m.sectionDefinitions() {
		sum += heights[def.id]
	}
	if sum > totalH {
		deficit := sum - totalH
		shrinkOrder := []focusSection{
			sectionReading, sectionOku, sectionSearch, sectionIntro, sectionStats, sectionTimer,
		}
		for deficit > 0 {
			changed := false
			for _, id := range shrinkOrder {
				minH := 1
				if id == sectionReading || id == sectionOku {
					minH = 2
				}
				if heights[id] > minH {
					heights[id]--
					deficit--
					changed = true
					if deficit == 0 {
						break
					}
				}
			}
			if !changed {
				break
			}
		}
	}
	sum = 0
	for _, def := range m.sectionDefinitions() {
		sum += heights[def.id]
	}
	if sum < totalH {
		heights[sectionReading] += totalH - sum
	}

	return heights
}

func (m *Model) renderSectionCard(def sectionDef, w, h int, focused bool) string {
	if h <= 0 {
		return ""
	}
	label := m.formatSectionLabel(def.id, def.label, def.count, focused)
	if h < 3 {
		// Too short to draw a border around: just the label.
		return clampPanelContent(label, w, h)
	}
	innerH := h - 2

	content := label
	if def.id == sectionReading || def.id == sectionOku || def.id == sectionSearch {
		// innerH > 1 leaves at least one row under the label.
		if innerH > 1 {
			if body := m.sectionContent(def.id, max(8, w-4), innerH-1); body != "" {
				content += "\n" + body
			}
		}
	}

	style := m.st.pane
	if focused {
		style = m.st.paneFocused
	}
	// A list whose items are taller than the rows it was given renders past
	// them; clip so the overflow cannot push the cards below this one off the
	// panel.
	clipped := clampPanelContent(content, w, innerH)
	clipped = stampOverflowBadge(clipped, m.listOverflowBadge(def.id), w, m.st)
	return style.Width(w).Height(innerH).Render(clipped)
}

// listOverflowBadge is the library section's badge, or nothing for the
// other cards.
func (m *Model) listOverflowBadge(id focusSection) string {
	if lib, ok := m.sections[id].(*librarySection); ok {
		return lib.overflowBadge()
	}
	return ""
}

// stampOverflowBadge right-aligns the badge on the card's last row, in the
// space the pagination dots used to take. The row is overwritten rather than
// appended to: a list pads its rows out to the full card width, so there is
// never anything left to append to.
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

func (m *Model) formatSectionLabel(id focusSection, label string, count int, focused bool) string {
	num := fmt.Sprintf("%d", int(id)+1)
	countStr := ""
	if count >= 0 {
		countStr = m.st.sectionCountLabel.Render(fmt.Sprintf(" (%d)", count))
	}

	// Timer running indicator.
	if id == sectionTimer && m.shared.timer != nil {
		elapsed := m.shared.now().Sub(m.shared.timer.StartedAt)
		countStr = " " + m.st.keyHint.Render(format.Duration(elapsed))
	}

	if focused {
		return m.st.sectionLabelFocused.Render("▸ "+num+"  "+label) + countStr
	}
	return m.st.sectionLabel.Render("  "+num+"  "+label) + countStr
}

// sectionContent returns the expanded content for a card: the list, or the
// search input row.
func (m *Model) sectionContent(id focusSection, w, h int) string {
	switch id {
	case sectionReading, sectionOku:
		return m.sections[id].View(w, h)
	case sectionSearch:
		return m.search.inputRow()
	default:
		// Intro, Stats, Timer use the right pane for full details.
		return m.st.dim.Render("  See Output panel")
	}
}

// ── Right Panel Views ──────────────────────────────────────────────────────

func (m *Model) rightPanelView(w, h int) string {
	switch m.tab {
	case sectionReading, sectionOku:
		return detailsView(m.section().Selected().Book, m.shared.density, w, m.st)
	case sectionTimer:
		if p := m.timerPicker(); p != nil {
			return p.View(m.lay, m.st)
		}
		return m.section().View(w, h)
	default:
		return m.section().View(w, h)
	}
}

// ── Resize ─────────────────────────────────────────────────────────────────

// resize pushes the layout's sizes into the sections and the modals.
// leftSectionHeights gives the focused list extra rows, so this follows the
// focus and not only a window resize.
func (m *Model) resize() {
	m.help.Width = m.helpBarWidth()

	totalW := max(60, m.lay.W-2)
	panelInnerH := m.rightPanelContentHeight()

	leftW := max(28, totalW*2/5)
	rightW := max(28, totalW-leftW-3)

	heights := m.leftSectionHeights(panelInnerH)
	readingContentH := max(1, heights[sectionReading]-3)
	okuContentH := max(1, heights[sectionOku]-3)
	leftContentW := leftW - 6
	if leftContentW < 8 {
		leftContentW = 8
	}

	// "[NORMAL] [BOOK] / " eats the front of the search card's row; the input
	// takes what is left instead of being cut off mid-placeholder.
	m.search.resizeInput(max(4, leftContentW-20))

	m.sections[sectionReading].Resize(leftContentW, readingContentH)
	m.sections[sectionOku].Resize(leftContentW, okuContentH)
	m.search.Resize(rightW-4, max(1, panelInnerH-1))
	rightInner := m.lay.rightPanelContentWidth()
	m.sections[sectionIntro].Resize(rightInner, panelInnerH)
	m.sections[sectionStats].Resize(rightInner, panelInnerH)
	m.sections[sectionTimer].Resize(rightInner, panelInnerH)

	for _, mod := range m.modals {
		mod.Resize(m.lay)
	}
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

// ── Progress Bar ────────────────────────────────────────────────────────────

// progressBar renders a Unicode block-character progress bar.
//
//	progressBar(45, 300, 20) → "███░░░░░░░░░░░░░░░░░  15%"
func progressBar(current, total, width int, st styles) string {
	if total <= 0 {
		return st.dim.Render(fmt.Sprintf("p.%d", current))
	}
	pct := float64(current) / float64(total)
	if pct > 1.0 {
		pct = 1.0
	}
	if pct < 0 {
		pct = 0
	}
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	bar := st.progressFilled.Render(strings.Repeat("█", filled)) +
		st.progressEmpty.Render(strings.Repeat("░", empty))

	pctStr := fmt.Sprintf("%3d%%", int(pct*100))
	return fmt.Sprintf("%s %s", bar, st.dim.Render(pctStr))
}

// miniProgressBar renders a compact progress bar for inline list items.
func miniProgressBar(current, total, width int) string {
	if total <= 0 {
		return ""
	}
	pct := float64(current) / float64(total)
	if pct > 1.0 {
		pct = 1.0
	}
	if pct < 0 {
		pct = 0
	}
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	empty := width - filled

	return strings.Repeat("█", filled) + strings.Repeat("░", empty) +
		fmt.Sprintf(" %d%%", int(pct*100))
}
