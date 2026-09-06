package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// statsModel is a loaded dashboard on the Stats tab, short enough that the
// page overflows the pane and there is something to scroll.
func statsModel(t *testing.T, w, h int) *Model {
	t.Helper()
	m := renderedDashboard(w, h)
	m.shared.stats, m.shared.sessions = demoLocalData(fixedNow)
	m.shared.weekly = m.shared.stats.Weekly
	m.broadcast(dataChangedMsg{dataLocal})
	m.setTab(tabStats)
	m.frame() // fills the viewport, which is what the keys scroll
	if s := statsOf(m); s.vp.TotalLineCount() <= s.vp.Height {
		t.Fatalf("the stats page fits in %dx%d, so this test proves nothing", w, h)
	}
	return m
}

// TestStatsScrollKeys walks the page's keys: a line, half a page, and the
// two ends. The page used to be clipped by hand and only j, k and g worked.
func TestStatsScrollKeys(t *testing.T) {
	m := statsModel(t, 80, 24)
	s := statsOf(m)
	bottom := s.vp.TotalLineCount() - s.vp.Height

	send(t, m, runeKey('j'))
	if s.vp.YOffset != 1 {
		t.Fatalf("offset after j = %d, want 1", s.vp.YOffset)
	}
	send(t, m, runeKey('k'))
	if s.vp.YOffset != 0 {
		t.Fatalf("offset after k = %d, want 0", s.vp.YOffset)
	}

	send(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	half := s.vp.YOffset
	if half <= 1 {
		t.Fatalf("offset after ctrl+d = %d, want half a page", half)
	}
	send(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if s.vp.YOffset != 0 {
		t.Fatalf("offset after ctrl+u = %d, want back to the top", s.vp.YOffset)
	}

	send(t, m, runeKey('G'))
	if s.vp.YOffset != bottom {
		t.Fatalf("offset after G = %d, want the last page (%d)", s.vp.YOffset, bottom)
	}
	send(t, m, runeKey('g'))
	if s.vp.YOffset != 0 {
		t.Fatalf("offset after g = %d, want the top", s.vp.YOffset)
	}
}

// TestStatsKeepsItsScrollAcrossAReload is the detail pane's rule for the
// stats page: a background reload or a finished timer rebuilds it, and must
// not throw the reader back to the top of it.
func TestStatsKeepsItsScrollAcrossAReload(t *testing.T) {
	m := statsModel(t, 80, 24)
	s := statsOf(m)
	send(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	scrolled := s.vp.YOffset

	m.broadcast(dataChangedMsg{dataLocal})
	m.frame()

	if s.vp.YOffset != scrolled {
		t.Fatalf("offset after a reload = %d, want the reader kept at %d", s.vp.YOffset, scrolled)
	}
}

// TestStatsPageIsBuiltOnlyWhenSomethingChanged pins the memo: a dozen charts
// are not rebuilt on every frame, and a data change is what drops them.
func TestStatsPageIsBuiltOnlyWhenSomethingChanged(t *testing.T) {
	m := statsModel(t, 80, 24)
	before := stripANSI(m.frame())

	// Mutated behind the memo's back: the page it already holds stands.
	m.shared.stats.Year.BooksFinished += 7
	if stripANSI(m.frame()) != before {
		t.Fatal("the page was rebuilt with nothing to say it had changed")
	}

	m.broadcast(dataChangedMsg{dataLocal})
	if stripANSI(m.frame()) == before {
		t.Fatal("a data change should drop the memoised page")
	}
}

// TestStatsShowsHowFarDownThePageIs pins the scroll indicator to the pane's
// last row, and to the page actually being longer than the box.
func TestStatsShowsHowFarDownThePageIs(t *testing.T) {
	m := statsModel(t, 80, 24)
	s := statsOf(m)
	rows := strings.Split(stripANSI(s.View(m.lay.ContentInner, m.lay.InnerH)), "\n")
	last := rows[len(rows)-1]

	want := fmt.Sprintf("▲ %d/%d ▼", s.vp.Height, s.vp.TotalLineCount())
	if !strings.HasSuffix(strings.TrimRight(last, " "), want) {
		t.Fatalf("the last row is %q, want it to end with %q", last, want)
	}

	// A page that fits carries no badge.
	tall := renderedDashboard(80, 60)
	tall.setTab(tabStats)
	if got := stripANSI(statsOf(tall).View(tall.lay.ContentInner, tall.lay.InnerH)); strings.Contains(got, "▲") {
		t.Fatalf("a page that fits should not claim to overflow:\n%s", got)
	}
}

// TestStatsBarsGrowWithThePane pins the charts to the width they are drawn
// into: they were a fixed ten cells wide whatever the terminal was.
func TestStatsBarsGrowWithThePane(t *testing.T) {
	narrow := statsOf(statsModel(t, 80, 24))
	wide := statsOf(statsModel(t, 160, 40))

	run := func(s *statsSection) int {
		longest := 0
		for _, line := range strings.Split(stripANSI(s.render(s.w)), "\n") {
			if n := strings.Count(line, "█") + strings.Count(line, "░"); n > longest {
				longest = n
			}
		}
		return longest
	}
	if n, w := run(narrow), run(wide); w <= n {
		t.Fatalf("the widest bar is %d cells at 80 columns and %d at 160: the charts do not follow the pane", n, w)
	}

	// Whatever the width, no row may push the pane wider than its box.
	for _, s := range []*statsSection{narrow, wide} {
		for i, line := range strings.Split(stripANSI(s.render(s.w)), "\n") {
			if got := lipgloss.Width(line); got > s.w {
				t.Fatalf("row %d is %d wide in a %d-column pane: %q", i, got, s.w, line)
			}
		}
	}
}
