package charts

import (
	"strings"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

func TestHeatmapLevel(t *testing.T) {
	cases := []struct {
		name string
		act  model.DayActivity
		max  int
		want int
	}{
		{"no activity", model.DayActivity{}, 100, 0},
		{"journal only, one entry", model.DayActivity{Entries: 1, HasActivity: true}, 100, 1},
		{"journal only, few entries", model.DayActivity{Entries: 3, HasActivity: true}, 100, 2},
		{"journal only, busy day", model.DayActivity{Entries: 12, HasActivity: true}, 100, 4},
		{"legacy activity flag without count", model.DayActivity{HasActivity: true}, 100, 1},
		{"low minutes", model.DayActivity{Minutes: 10}, 100, 1},
		{"max minutes", model.DayActivity{Minutes: 100}, 100, 4},
		{"minutes beat entries", model.DayActivity{Minutes: 100, Entries: 1, HasActivity: true}, 100, 4},
		{"entries beat minutes", model.DayActivity{Minutes: 10, Entries: 6, HasActivity: true}, 100, 4},
	}
	for _, c := range cases {
		if got := HeatmapLevel(c.act, c.max); got != c.want {
			t.Errorf("%s: HeatmapLevel = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestBuildHeatmapShowsAllWeekdays(t *testing.T) {
	// One journal-only day per weekday over the last full week.
	now := time.Now()
	weekday := (int(now.Weekday()) + 6) % 7 // Mon=0 .. Sun=6
	monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, -weekday-7)

	var acts []model.DayActivity
	for i := 0; i < 7; i++ {
		acts = append(acts, model.DayActivity{
			Date:        monday.AddDate(0, 0, i),
			Entries:     1,
			HasActivity: true,
		})
	}

	out := Heatmap(acts, 4, Plain)
	lines := strings.Split(out, "\n")

	// Month row + 7 weekday rows + blank + legend.
	if len(lines) != 10 {
		t.Fatalf("got %d lines, want 10:\n%s", len(lines), out)
	}
	for i, label := range []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"} {
		row := lines[1+i]
		if !strings.HasPrefix(row, "  "+label+"  ") {
			t.Errorf("row %d = %q, want prefix %q", i, row, "  "+label+"  ")
		}
		if !strings.Contains(row, "░") {
			t.Errorf("row %s has no active cell: %q", label, row)
		}
	}
}

// heatmapMonday returns the Monday that starts the week containing d.
func heatmapMonday(d time.Time) time.Time {
	weekday := (int(d.Weekday()) + 6) % 7
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location()).AddDate(0, 0, -weekday)
}

func TestBuildHeatmapMonthLabelAlignsWithColumns(t *testing.T) {
	const weeks = 26
	cases := []struct {
		name string
		now  time.Time
	}{
		{"month starts on Tuesday", time.Date(2026, time.September, 4, 12, 0, 0, 0, time.Local)},
		{"month starts on Monday", time.Date(2026, time.June, 15, 12, 0, 0, 0, time.Local)},
		{"month starts on Sunday", time.Date(2026, time.March, 1, 12, 0, 0, 0, time.Local)},
		{"today is the 1st", time.Date(2026, time.October, 1, 12, 0, 0, 0, time.Local)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := HeatmapAt(nil, weeks, c.now, Plain)
			monthRow := strings.Split(out, "\n")[0]
			gridW := HeatmapPrefixW + weeks*HeatmapWeekW
			if len([]rune(monthRow)) > gridW+len("Jan") {
				t.Fatalf("month row wider than grid: %q", monthRow)
			}

			// Every month whose 1st lies inside the grid must be labelled on
			// the column whose Mon–Sun span contains that 1st — including the
			// current month, even when it started mid-week.
			start := heatmapMonday(c.now).AddDate(0, 0, -(weeks-1)*7)
			first := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, start.Location())
			for first.Before(start) {
				first = first.AddDate(0, 1, 0)
			}
			labelled := 0
			for !first.After(c.now) {
				week := 0
				for col := start; heatmapMonday(first).After(col); col = col.AddDate(0, 0, 7) {
					week++
				}
				pos := HeatmapPrefixW + week*HeatmapWeekW
				name := first.Format("Jan")
				if got := string([]rune(monthRow)[pos : pos+len(name)]); got != name {
					t.Errorf("%s: want %q at col %d (week %d), month row %q", first.Format("2006-01-02"), name, pos, week, monthRow)
				}
				labelled++
				first = first.AddDate(0, 1, 0)
			}
			if labelled == 0 {
				t.Fatal("test covered no month boundaries")
			}
			if !strings.Contains(monthRow, c.now.Format("Jan")) {
				t.Errorf("month row %q missing current month %q", monthRow, c.now.Format("Jan"))
			}
		})
	}
}

func TestFitHeatmapWeeks(t *testing.T) {
	cases := []struct {
		weeks, width, want int
	}{
		// Room for more than was asked for: the request stands.
		{26, 200, 26},
		// Exactly enough: the gutter, two columns a week, and two of slack.
		{26, HeatmapPrefixW + 26*HeatmapWeekW + 2, 26},
		// One column short.
		{26, HeatmapPrefixW + 26*HeatmapWeekW + 1, 25},
		// A narrow pane still gets a stub of a grid rather than nothing.
		{26, 10, 4},
		{26, 0, 4},
	}
	for _, c := range cases {
		if got := FitHeatmapWeeks(c.weeks, c.width); got != c.want {
			t.Errorf("FitHeatmapWeeks(%d, %d) = %d, want %d", c.weeks, c.width, got, c.want)
		}
	}
}
