package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// tab is one page of the dashboard. The values are the order of the header
// strip, and their numbers (1-based) are the keys that jump to them.
type tab int

const (
	tabReading tab = iota
	tabOku
	tabSearch
	tabStats
	tabTimer
	tabCount // sentinel
)

// hasDetail reports whether the tab has a selection worth a pane of its own.
// Stats and Timer are pages in their own right and take the full width.
func (t tab) hasDetail() bool {
	return t == tabReading || t == tabOku || t == tabSearch
}

// name is the tab's word in the header strip. The pane's own title says more
// (the count, the year); this is only the label under its number.
func (t tab) name() string {
	switch t {
	case tabOku:
		return "Oku"
	case tabSearch:
		return "Search"
	case tabStats:
		return "Stats"
	case tabTimer:
		return "Timer"
	default:
		return "Reading"
	}
}

// tabForKey maps a key press to the tab it jumps to: "1" is the first.
func tabForKey(name string) (tab, bool) {
	if len(name) != 1 || name[0] < '1' || name[0] > '0'+byte(tabCount) {
		return 0, false
	}
	return tab(name[0] - '1'), true
}

// ── Header ─────────────────────────────────────────────────────────────────

// headerGap is the smallest run of spaces between the tab strip and what is
// right-aligned after it, so the two never read as one word.
const headerGap = 2

// header is the top row: the app name and the spinner on the left, the
// numbered tab strip after it, and — as far right as they fit — the running
// timer and how long ago the library was synced.
//
// The strip is what the header is for, so it is the last thing to be cut:
// the timer drops its book title first, then the sync age goes, then the
// timer itself.
func (m *Model) header(lay layout) string {
	st := m.st
	w := max(minFrameWidth, lay.W)

	left := " " + st.headerTitle.Render("oku")
	if m.isLoading() {
		left += " " + st.headerAccent.Render(m.shared.spin.View())
	}
	strip := m.tabStrip()

	// The room left for the right-hand side, once the strip has had its own.
	room := w - lipgloss.Width(left) - lipgloss.Width(strip) - headerGap - 1
	right := m.headerStatus(room)

	if room < 0 {
		// Not even the strip fits: cut it rather than wrap the row.
		strip = ansi.Truncate(strip, max(0, w-lipgloss.Width(left)-1), "…")
	}

	gap := max(headerGap, w-lipgloss.Width(left)-lipgloss.Width(strip)-lipgloss.Width(right)-1)
	row := left + strip + st.headerFill.Render(strings.Repeat(" ", gap)) + right
	return st.headerBar.Width(w).MaxHeight(1).Render(ansi.Truncate(row, w-1, ""))
}

// tabStrip is the numbered strip: every tab, its count when it has one, and
// a marker on the active one so the focus survives a terminal without colour.
func (m *Model) tabStrip() string {
	st := m.st
	parts := make([]string, 0, tabCount)
	for t := tab(0); t < tabCount; t++ {
		marker, label := " ", st.tabIdle
		if t == m.tab {
			marker, label = "▸", st.tabActive
		}
		seg := label.Render(fmt.Sprintf("%s%d %s", marker, int(t)+1, t.name()))
		if n := m.tabCount(t); n >= 0 {
			seg += st.tabCount.Render(fmt.Sprintf(" %d", n))
		}
		parts = append(parts, seg)
	}
	return "  " + strings.Join(parts, "  ")
}

// tabCount is the number shown next to a tab in the strip, or -1 for the
// tabs that are a page rather than a list.
func (m *Model) tabCount(t tab) int {
	switch t {
	case tabReading:
		return len(m.shared.reading)
	case tabOku:
		return len(m.shared.oku)
	}
	return -1
}

// headerStatus is the right-hand end of the header: the running timer and
// the sync age, in the longest form that fits room columns. It returns ""
// rather than something cut mid-word.
func (m *Model) headerStatus(room int) string {
	st := m.st
	full, short := m.timerBadge()
	sync := m.syncBadge()
	syncShort := strings.TrimPrefix(sync, "synced ")

	render := func(parts ...string) string {
		kept := make([]string, 0, len(parts))
		for _, p := range parts {
			if p != "" {
				kept = append(kept, p)
			}
		}
		return strings.Join(kept, st.headerFill.Render("  "))
	}
	for _, candidate := range []string{
		render(st.headerAccent.Render(full), st.headerDim.Render(sync)),
		render(st.headerAccent.Render(short), st.headerDim.Render(sync)),
		render(st.headerAccent.Render(short), st.headerDim.Render(syncShort)),
		render(st.headerAccent.Render(short)),
		render(st.headerDim.Render(syncShort)),
	} {
		if strings.TrimSpace(ansi.Strip(candidate)) != "" && lipgloss.Width(candidate) <= room {
			return candidate
		}
	}
	return ""
}

// timerBadge is the running timer, with and without the book's title. Both
// are empty when no timer runs.
func (m *Model) timerBadge() (full, short string) {
	if m.shared.timer == nil {
		return "", ""
	}
	elapsed := m.shared.now().Sub(m.shared.timer.StartedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	h := int(elapsed.Hours())
	min := int(elapsed.Minutes()) % 60
	sec := int(elapsed.Seconds()) % 60
	short = fmt.Sprintf("▶ %02d:%02d:%02d", h, min, sec)

	full = short
	if m.shared.timerBook != nil && m.shared.timerBook.Title != "" {
		full = short + " " + ansi.Truncate(m.shared.timerBook.Title, 30, "…")
	}
	return full, short
}

// syncBadge says when the library was last refreshed from Hardcover.
func (m *Model) syncBadge() string {
	if m.syncing {
		return "syncing…"
	}
	return syncLabel(m.shared.lastSyncAt, m.shared.now())
}

// syncLabel renders how long ago a sync finished, at the coarsest unit that
// still says something.
func syncLabel(last, now time.Time) string {
	if last.IsZero() {
		return "never synced"
	}
	d := now.Sub(last)
	switch {
	case d < time.Minute:
		return "synced just now"
	case d < time.Hour:
		return fmt.Sprintf("synced %dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("synced %dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("synced %dd ago", int(d.Hours()/24))
	}
}

// ── Footer ─────────────────────────────────────────────────────────────────

// footer is the bottom row: the hints for whatever has focus on the left,
// and the toast right-aligned after them. The toast has priority — it is the
// only report a finished operation gets — so the hints are cut to what is
// left, whole hint by whole hint, and the help hint is never the one dropped.
func (m *Model) footer(lay layout) string {
	w := max(minFrameWidth, lay.W)

	toast := m.renderToast(max(0, w-2-minHelpBarWidth))
	toastW := lipgloss.Width(toast)

	limit := w - 2
	if toastW > 0 {
		limit -= toastW + 2
	}
	bar := m.renderHelpBar(m.helpBindings(), max(minHelpBarWidth, limit))

	row := " " + bar
	if toastW > 0 {
		gap := max(1, w-1-lipgloss.Width(bar)-toastW-1)
		row += strings.Repeat(" ", gap) + toast
	}
	return ansi.Truncate(row, w, "")
}
