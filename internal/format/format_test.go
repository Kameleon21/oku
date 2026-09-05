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
