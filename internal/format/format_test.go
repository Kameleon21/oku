package format

import (
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

func TestDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{9 * time.Second, "9s"},
		{90 * time.Second, "1m 30s"},
		{59*time.Minute + 59*time.Second, "59m 59s"},
		{2*time.Hour + 5*time.Minute, "2h 5m"},
		{25*time.Hour + 1*time.Minute + 30*time.Second, "25h 1m"},
		// Sub-second input rounds before it is split, so 1999ms is "2s".
		{1999 * time.Millisecond, "2s"},
	}
	for _, c := range cases {
		if got := Duration(c.in); got != c.want {
			t.Errorf("Duration(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCount(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.0K"},
		{1234, "1.2K"},
		{999_999, "1000.0K"},
		{1_000_000, "1.0M"},
		{4_200_000, "4.2M"},
	}
	for _, c := range cases {
		if got := Count(c.in); got != c.want {
			t.Errorf("Count(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestThousands(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1748, "1,748"},
		{12345, "12,345"},
		{1234567, "1,234,567"},
	}
	for _, c := range cases {
		if got := Thousands(c.in); got != c.want {
			t.Errorf("Thousands(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClock(t *testing.T) {
	// A zone the test controls, so "20:12 local" is not the machine's guess.
	tokyo := time.FixedZone("JST", 9*60*60)
	at := time.Date(2026, time.September, 5, 20, 12, 44, 0, tokyo)
	want := at.Local().Format("15:04")
	if got := Clock(at); got != want {
		t.Errorf("Clock = %q, want %q", got, want)
	}
	// Whatever zone the test runs in, the shape is fixed.
	if got := Clock(time.Date(2026, time.September, 5, 20, 12, 0, 0, time.Local)); got != "20:12" {
		t.Errorf("Clock(20:12 local) = %q, want %q", got, "20:12")
	}
	if got := Clock(time.Date(2026, time.September, 5, 9, 5, 0, 0, time.Local)); got != "09:05" {
		t.Errorf("Clock(09:05 local) = %q, want %q", got, "09:05")
	}
}

func TestDayLabel(t *testing.T) {
	now := time.Date(2026, time.September, 5, 20, 30, 0, 0, time.Local)
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{"same day", time.Date(2026, time.September, 5, 8, 0, 0, 0, time.Local), "Today"},
		{"one minute past local midnight", time.Date(2026, time.September, 5, 0, 1, 0, 0, time.Local), "Today"},
		{"ten to midnight yesterday", time.Date(2026, time.September, 4, 23, 50, 0, 0, time.Local), "Yest."},
		{"two days back", time.Date(2026, time.September, 3, 21, 12, 0, 0, time.Local), "Sep 03"},
		{"last year", time.Date(2025, time.December, 31, 12, 0, 0, 0, time.Local), "Dec 31"},
	}
	for _, c := range cases {
		if got := DayLabel(c.at, now); got != c.want {
			t.Errorf("%s: DayLabel = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestDayLabelBucketsInLocalTime pins the bucketing to the reader's own
// midnight. An evening in the Americas has already rolled over into the next
// UTC day; it is still "Today" to the person who read it.
func TestDayLabelBucketsInLocalTime(t *testing.T) {
	prev := time.Local
	time.Local = time.FixedZone("UTC-4", -4*60*60)
	t.Cleanup(func() { time.Local = prev })

	now := time.Date(2026, time.September, 5, 22, 30, 0, 0, time.Local)
	at := time.Date(2026, time.September, 5, 21, 0, 0, 0, time.Local)
	if at.UTC().Day() == at.Day() {
		t.Fatal("fixture does not straddle a UTC midnight")
	}

	if got := DayLabel(at, now); got != "Today" {
		t.Errorf("DayLabel = %q, want %q (bucketed in UTC, not local)", got, "Today")
	}
	if got := DayLabel(at.AddDate(0, 0, -1), now); got != "Yest." {
		t.Errorf("DayLabel for the evening before = %q, want %q", got, "Yest.")
	}
	// The clock follows the reader too: 21:00 local, not 01:00 UTC.
	if got := Clock(at); got != "21:00" {
		t.Errorf("Clock = %q, want %q", got, "21:00")
	}
}

func TestBookMeta(t *testing.T) {
	cases := []struct {
		name string
		book model.Book
		want string
	}{
		{"nothing known", model.Book{Title: "Dune"}, ""},
		{
			"rating without a count",
			model.Book{Rating: 4.3},
			"★ 4.30",
		},
		{
			"everything",
			model.Book{
				Rating: 4.31, RatingsCount: 1234, UsersReadCount: 4200,
				ReleaseDate: "1965", FeaturedSeries: "Dune", FeaturedSeriesPosition: 1,
			},
			"★ 4.31 (1.2K ratings) · 4.2K readers · released 1965 · series: Dune #1",
		},
		{
			"series without a position",
			model.Book{FeaturedSeries: "Dune"},
			"series: Dune",
		},
	}
	for _, c := range cases {
		if got := BookMeta(c.book); got != c.want {
			t.Errorf("%s: BookMeta = %q, want %q", c.name, got, c.want)
		}
	}
}
