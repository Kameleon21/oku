package format

import (
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// TestReadPace pins the estimate the detail pane prints. The pace assumes
// the read started at page 0 (see model.ReadPace), so these are the numbers
// that assumption gives. 5 Sep 2026 is a Saturday.
func TestReadPace(t *testing.T) {
	now := time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local)
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }

	cases := []struct {
		name        string
		started     time.Time
		page, pages int
		want        string
	}{
		{"started today has no pace yet", now.Add(-3 * time.Hour), 40, 300, "started today"},
		{"done tomorrow", day(5), 100, 120, "20 pages/day · ~1 day left (≈ tomorrow)"},
		{"done this week names the day", day(4), 120, 210, "30 pages/day · ~3 days left (≈ Tuesday)"},
		{"a day in", day(1), 30, 300, "30 pages/day · ~9 days left (≈ 14 Sep)"},
		{"a week in, rounded to weeks", day(7), 70, 300, "10 pages/day · ~3 weeks left (≈ 28 Sep)"},
		{"months out", day(24), 70, 305, "3 pages/day · ~3 months left (≈ 25 Nov)"},
		{"a page a day is singular", day(20), 20, 40, "1 page/day · ~3 weeks left (≈ 25 Sep)"},
		{"under a page a day, still estimable", day(30), 20, 300, "<1 page/day · ~14 months left (≈ 30 Oct 2027)"},
		{"finished books get the pace only", day(10), 300, 300, "30 pages/day"},
		{"too slow to estimate", day(400), 20, 3000, "<1 page/day · pace too low to estimate"},
		{"past two years is not an estimate", day(2), 2, 3000, "1 page/day"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := model.ForecastRead(c.started, now, c.page, c.pages)
			if got := ReadPace(p, now); got != c.want {
				t.Fatalf("ReadPace = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFinishDay(t *testing.T) {
	now := time.Date(2026, 12, 26, 22, 0, 0, 0, time.Local) // a Saturday
	at := func(days int) time.Time { return time.Date(2026, 12, 26+days, 0, 0, 0, 0, time.Local) }
	cases := []struct {
		days int
		want string
	}{
		{0, "today"},
		{1, "tomorrow"},
		{2, "Monday"},
		{6, "Friday"},
		{7, "2 Jan 2027"},
		{-30, "today"},
	}
	for _, c := range cases {
		if got := FinishDay(at(c.days), now); got != c.want {
			t.Errorf("FinishDay(+%d) = %q, want %q", c.days, got, c.want)
		}
	}
	if got := FinishDay(time.Date(2026, 12, 20, 0, 0, 0, 0, time.Local), time.Date(2026, 12, 1, 9, 0, 0, 0, time.Local)); got != "20 Dec" {
		t.Errorf("a date this year = %q, want %q", got, "20 Dec")
	}
}

// TestGoalPace pins the goal line. The window is 1 Jan to 11 Mar, so 5 Feb
// is halfway with five weeks to go and 7 Mar leaves five days.
func TestGoalPace(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	halfway := time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)
	lastWeek := time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		metric   string
		target   int
		progress float64
		now      time.Time
		want     string
	}{
		{"behind", "books", 20, 8, halfway, "2 books behind · 2.4 books a week gets you to 20"},
		{"one behind is singular", "books", 20, 9, halfway, "1 book behind · 2.2 books a week gets you to 20"},
		{"on track", "books", 20, 10, halfway, "on track · 2 books a week gets you to 20"},
		{"a book a week is singular", "books", 10, 5, halfway, "on track · 1 book a week gets you to 10"},
		{"ahead says where the pace lands", "books", 20, 13, halfway, "3 books ahead · on pace for 26"},
		{"the last week counts what is left", "books", 20, 17, lastWeek, "2 books behind · 3 more books by 11 Mar"},
		{"reached", "books", 20, 20, halfway, "goal reached"},
		{"float noise does not round up a tenth", "books", 21, 7.5, halfway, "3 books behind · 2.7 books a week gets you to 21"},
		{"a slow pages goal is still whole pages", "pages", 40, 18, halfway, "2 pages behind · 5 pages a week gets you to 40"},
		{"a pages goal rounds to whole pages", "pages", 10000, 4000, halfway, "1,000 pages behind · 1,200 pages a week gets you to 10,000"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := model.Goal{Metric: c.metric, Target: c.target, Progress: c.progress, StartDate: start, EndDate: end}
			p, ok := model.ForecastGoal(g, c.now)
			if !ok {
				t.Fatal("goal did not forecast")
			}
			if got := GoalPace(p); got != c.want {
				t.Fatalf("GoalPace = %q, want %q", got, c.want)
			}
		})
	}
}
