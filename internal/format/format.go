// Package format renders the small values the CLI and the dashboard both
// print: durations, large counts and a book's metadata line. They live here so
// `oku timer` and the TUI cannot drift apart, and so neither has to import the
// other.
package format

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// Duration renders a duration at the coarsest unit that still says something:
//
//	Duration(2*time.Hour + 5*time.Minute) → "2h 5m"
//	Duration(90*time.Second)              → "1m 30s"
//	Duration(9*time.Second)               → "9s"
func Duration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// Count abbreviates a large count so it fits a list row: 1234 → "1.2K".
func Count(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// Thousands formats an int with thousands separators: 1748 → "1,748".
func Thousands(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var sb strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		sb.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if sb.Len() > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(s[i : i+3])
	}
	return sb.String()
}

// Clock is the time of day a session started, in the reader's own zone:
// 24-hour, no seconds. Sessions are listed one per row against a date, so the
// hour is all that distinguishes them.
func Clock(t time.Time) string {
	return t.Local().Format("15:04")
}

// DayLabel names the day t falls on, relative to now: "Today", "Yest.", or the
// date. Both are bucketed to local midnight, so a session at 23:50 and one at
// 00:10 read as different days even though they are twenty minutes apart, and
// a session logged late in the evening is not "yesterday" because UTC has
// already rolled over.
func DayLabel(t, now time.Time) string {
	day := midnight(t.Local())
	today := midnight(now.Local())
	switch {
	case day.Equal(today):
		return "Today"
	case day.Equal(today.AddDate(0, 0, -1)):
		return "Yest."
	default:
		// Day before month, so the two variable digits come first and the
		// column reads down the page: "3 Sep", "31 Dec".
		return t.Local().Format("2 Jan")
	}
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// BookMeta is the one-line summary under a book: rating, readers, release date
// and series, in that order, joined by " · ". Parts with no data are omitted,
// so an unrated book gets a shorter line rather than an empty one.
func BookMeta(b model.Book) string {
	parts := make([]string, 0, 4)

	if b.Rating > 0 {
		rating := fmt.Sprintf("★ %.2f", b.Rating)
		if b.RatingsCount > 0 {
			rating += fmt.Sprintf(" (%s ratings)", Count(b.RatingsCount))
		}
		parts = append(parts, rating)
	}

	if b.UsersReadCount > 0 {
		parts = append(parts, fmt.Sprintf("%s readers", Count(b.UsersReadCount)))
	}

	if b.ReleaseDate != "" {
		parts = append(parts, "released "+b.ReleaseDate)
	}

	if b.FeaturedSeries != "" {
		series := "series: " + b.FeaturedSeries
		if b.FeaturedSeriesPosition > 0 {
			series += fmt.Sprintf(" #%d", b.FeaturedSeriesPosition)
		}
		parts = append(parts, series)
	}

	return strings.Join(parts, " · ")
}
