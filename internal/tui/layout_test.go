package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// TestComputeLayout pins the geometry every pane is drawn from: where the
// split happens, what a pane's inner width is, and what Enter does on a
// terminal too narrow to show both panes at once.
func TestComputeLayout(t *testing.T) {
	cases := []struct {
		name          string
		w, h          int
		tab           tab
		detailFocused bool
		want          layout
	}{
		{
			name: "120x40 reading", w: 120, h: 40, tab: tabReading,
			want: layout{W: 120, H: 40, BodyH: 38, PaneH: 38, InnerH: 36,
				Split: true, ContentW: 48, DetailW: 72, ContentInner: 44, DetailInner: 68},
		},
		{
			name: "120x40 reading, detail focused: same boxes", w: 120, h: 40, tab: tabReading, detailFocused: true,
			want: layout{W: 120, H: 40, BodyH: 38, PaneH: 38, InnerH: 36,
				Split: true, ContentW: 48, DetailW: 72, ContentInner: 44, DetailInner: 68},
		},
		{
			name: "200x50 search", w: 200, h: 50, tab: tabSearch,
			want: layout{W: 200, H: 50, BodyH: 48, PaneH: 48, InnerH: 46,
				Split: true, ContentW: 80, DetailW: 120, ContentInner: 76, DetailInner: 116},
		},
		{
			name: "100x30 is the narrowest split", w: 100, h: 30, tab: tabOku,
			want: layout{W: 100, H: 30, BodyH: 28, PaneH: 28, InnerH: 26,
				Split: true, ContentW: 40, DetailW: 60, ContentInner: 36, DetailInner: 56},
		},
		{
			name: "99x30 is one column too narrow", w: 99, h: 30, tab: tabOku,
			want: layout{W: 99, H: 30, BodyH: 28, PaneH: 28, InnerH: 26,
				ContentW: 99, ContentInner: 95},
		},
		{
			name: "80x24 list", w: 80, h: 24, tab: tabReading,
			want: layout{W: 80, H: 24, BodyH: 22, PaneH: 22, InnerH: 20,
				ContentW: 80, ContentInner: 76},
		},
		{
			name: "80x24 with the detail focused is detail only", w: 80, h: 24, tab: tabReading, detailFocused: true,
			want: layout{W: 80, H: 24, BodyH: 22, PaneH: 22, InnerH: 20,
				DetailOnly: true, DetailW: 80, DetailInner: 76},
		},
		{
			name: "stats never splits", w: 120, h: 40, tab: tabStats,
			want: layout{W: 120, H: 40, BodyH: 38, PaneH: 38, InnerH: 36,
				ContentW: 120, ContentInner: 116},
		},
		{
			name: "timer never splits, and has no detail to focus", w: 120, h: 40, tab: tabTimer, detailFocused: true,
			want: layout{W: 120, H: 40, BodyH: 38, PaneH: 38, InnerH: 36,
				ContentW: 120, ContentInner: 116},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := computeLayout(c.w, c.h, c.tab, c.detailFocused)
			if got != c.want {
				t.Fatalf("computeLayout(%d, %d, %v, %v)\n got %+v\nwant %+v", c.w, c.h, c.tab, c.detailFocused, got, c.want)
			}
			// Whatever the case, the two panes take the whole width and
			// leave nothing over.
			if sum := got.ContentW + got.DetailW; sum != c.w {
				t.Fatalf("panes are %d wide together, want %d", sum, c.w)
			}
		})
	}
}

// TestLayoutIsPushedIntoThePanes checks that what computeLayout works out is
// what the section and the detail pane are actually sized to.
func TestLayoutIsPushedIntoThePanes(t *testing.T) {
	m := renderedDashboard(120, 40)
	if got, want := readingSection(m).list.Width(), m.lay.ContentInner; got != want {
		t.Fatalf("list width = %d, want the content pane's inner width %d", got, want)
	}
	if got, want := readingSection(m).list.Height(), m.lay.InnerH; got != want {
		t.Fatalf("list height = %d, want the pane's inner height %d", got, want)
	}
	if got, want := m.detail.vp.Width(), m.lay.DetailInner; got != want {
		t.Fatalf("detail width = %d, want %d", got, want)
	}

	// A narrower terminal moves both, and the mini bar shrinks with them.
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got, want := readingSection(m).list.Width(), m.lay.ContentInner; got != want {
		t.Fatalf("list width after resize = %d, want %d", got, want)
	}
	if got, want := readingSection(m).barWidth(), clampInt(m.lay.ContentInner-miniBarRoom, miniBarMin, miniBarMax); got != want {
		t.Fatalf("mini bar width = %d, want %d", got, want)
	}
}

// TestPaneFillsItsBoxExactly pins the one invariant the two-column layout
// rests on: every row of a pane is exactly as wide as the pane, so its
// neighbour starts where it should.
func TestPaneFillsItsBoxExactly(t *testing.T) {
	m := renderedDashboard(120, 40)
	for _, size := range [][2]int{{48, 38}, {72, 38}, {20, 4}, {6, 2}} {
		w, h := size[0], size[1]
		out := m.pane("A title long enough to be cut in a narrow pane", "one\ntwo\nthree", w, h, true)
		lines := strings.Split(out, "\n")
		if len(lines) != h {
			t.Fatalf("%dx%d pane has %d rows, want %d", w, h, len(lines), h)
		}
		for i, line := range lines {
			if got := lipgloss.Width(line); got != w {
				t.Fatalf("%dx%d pane row %d is %d wide, want %d: %q", w, h, i, got, w, stripANSI(line))
			}
		}
	}
}

// TestSectionViewsFillTheirBox checks that a section hands back exactly the
// rows it was given: the pane pads defensively, but a section that overflows
// would have its content silently cut instead.
func TestSectionViewsFillTheirBox(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}} {
		m := renderedDashboard(size[0], size[1])
		for _, tb := range allTabs {
			m.setTab(tb)
			w, h := m.lay.ContentInner, m.lay.InnerH
			view := m.section().View(w, h)
			lines := strings.Split(view, "\n")
			if len(lines) != h {
				t.Fatalf("%dx%d tab %v: View gave %d rows, want %d", size[0], size[1], tb, len(lines), h)
			}
			for i, line := range lines {
				if got := lipgloss.Width(line); got != w {
					t.Fatalf("%dx%d tab %v: row %d is %d wide, want %d", size[0], size[1], tb, i, got, w)
				}
			}
		}
	}
}
