package cli

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
		if got := heatmapLevel(c.act, c.max); got != c.want {
			t.Errorf("%s: heatmapLevel = %d, want %d", c.name, got, c.want)
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

	out := buildHeatmap(acts, 4, heatmapCellPlain, nil)
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

func TestBuildHeatmapMonthLabelAlignsWithColumns(t *testing.T) {
	out := buildHeatmap(nil, 26, heatmapCellPlain, nil)
	lines := strings.Split(out, "\n")

	// The current month's label must sit within the grid, over the final
	// weeks' columns (the old renderer drifted it past the right edge).
	monthRow := lines[0]
	name := time.Now().Format("Jan")
	pos := strings.LastIndex(monthRow, name)
	if pos < 0 {
		t.Fatalf("month row %q missing current month %q", monthRow, name)
	}
	gridW := heatmapPrefixW + 26*heatmapWeekW
	if pos >= gridW {
		t.Errorf("label %q at col %d, beyond grid width %d", name, pos, gridW)
	}
	// The label's column must correspond to a week whose Monday is in the
	// current month.
	week := (pos - heatmapPrefixW) / heatmapWeekW
	now := time.Now()
	weekday := (int(now.Weekday()) + 6) % 7
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).
		AddDate(0, 0, -25*7-weekday)
	if got := start.AddDate(0, 0, week*7).Month(); got != now.Month() {
		t.Errorf("label column maps to %s, want %s", got, now.Month())
	}
}
