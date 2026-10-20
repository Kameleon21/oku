package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/x/ansi"
)

func TestFinishingNext(t *testing.T) {
	now := time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local) // a Saturday
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }
	reading := func(id int, title string, started time.Time, page, pages int) model.UserBook {
		return model.UserBook{
			BookID:        id,
			StatusID:      model.StatusCurrentlyReading,
			Book:          model.Book{ID: id, Title: title, Pages: pages},
			UserBookReads: []model.UserBookRead{{ProgressPages: page, StartedAt: &started}},
		}
	}
	books := []model.UserBook{
		reading(1, "Software Architecture: The Hard Parts and Then Some", day(24), 70, 305),
		reading(2, "Dune", day(4), 120, 210),
		reading(3, "Unopened", day(3), 0, 300),
	}

	got := finishingNext(model.ForecastShelf(books, now), now)
	want := "\n  Finishing next\n\n" +
		"  Dune                              30 pages/day · ~3 days left (≈ Tuesday)\n" +
		"  Software Architecture: The Hard…  3 pages/day · ~3 months left (≈ 25 Nov)\n"
	if got != want {
		t.Fatalf("finishingNext =\n%s\nwant\n%s", got, want)
	}

	if got := finishingNext(nil, now); got != "" {
		t.Fatalf("no open reads should print nothing, got %q", got)
	}
}

func TestFinishingNextStopsAtTheLimit(t *testing.T) {
	now := time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local)
	reads := make([]model.BookForecast, finishingNextLimit+3)
	for i := range reads {
		reads[i] = model.BookForecast{Title: "Book", ReadPace: model.ReadPace{Days: 1, Page: 10, Pages: 100, PagesPerDay: 10}}
	}
	if rows := strings.Count(finishingNext(reads, now), "pages/day"); rows != finishingNextLimit {
		t.Fatalf("printed %d rows, want %d", rows, finishingNextLimit)
	}
}

// TestFinishingNextAlignsWideTitles: the column is measured in cells, so a
// title in a wide script lines up with the rest.
func TestFinishingNextAlignsWideTitles(t *testing.T) {
	now := time.Date(2026, 9, 5, 20, 30, 0, 0, time.Local)
	pace := model.ReadPace{Days: 1, Page: 10, Pages: 100, PagesPerDay: 10}
	out := finishingNext([]model.BookForecast{
		{Title: "ノルウェイの森", ReadPace: pace}, // seven runes, fourteen cells
		{Title: "Norwegian Wood", ReadPace: pace},
	}, now)
	rows := strings.Split(strings.TrimSpace(out), "\n")[1:]
	var cols []int
	for _, r := range rows {
		if r = strings.TrimSpace(r); r != "" {
			cols = append(cols, ansi.StringWidth(r[:strings.Index(r, "10 pages")]))
		}
	}
	if len(cols) != 2 || cols[0] != cols[1] {
		t.Fatalf("estimates start at columns %v, want them aligned:\n%s", cols, out)
	}
}
