package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// shouldUseDemoLocalData reports whether to show fabricated dashboard data.
// Opt-in only (for demo recordings): an empty database is a real state and a
// DB failure must surface as an error, never as fake numbers.
func shouldUseDemoLocalData() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("OKU_TUI_DEMO_DATA")), "1")
}

// demo is a whole dashboard's worth of fabricated data, built around one
// clock: the recording's sample library, and the fixtures the golden frames
// are rendered from.
type demo struct {
	reading, oku   []model.UserBook
	shelf          map[int]model.UserBook
	stats          *model.ReadingStats
	sessions       []model.ReadingSession
	results        []model.SearchResult
	recentSearches []string
}

// demoData fabricates everything the dashboard shows, as a pure function of
// now: the same clock gives the same page, which is what lets the golden
// frames be compared byte for byte.
func demoData(now time.Time) demo {
	stats, sessions := demoLocalData(now)
	reading, oku := demoLibrary(now)

	shelf := make(map[int]model.UserBook, len(reading)+len(oku)+1)
	for _, b := range append(append([]model.UserBook(nil), reading...), oku...) {
		shelf[b.Book.ID] = b
	}
	// A finished book, on no visible shelf: the search detail's "on your
	// shelf" row is the only place it shows.
	shelf[311] = model.UserBook{
		ID: 311, BookID: 311, StatusID: model.StatusRead, Rating: 4.5,
		Book: model.Book{ID: 311, Title: "Dune", Authors: []string{"Frank Herbert"}, Pages: 412},
	}

	return demo{
		reading:  reading,
		oku:      oku,
		shelf:    shelf,
		stats:    stats,
		sessions: sessions,
		results: []model.SearchResult{
			{ID: 311, Title: "Dune", Authors: []string{"Frank Herbert"}, Pages: 412, Slug: "dune", Rating: 4.31, Ratings: 1240},
			{ID: 312, Title: "Dune Messiah", Authors: []string{"Frank Herbert"}, Pages: 256, Slug: "dune-messiah", Rating: 3.90, Ratings: 410},
			{ID: 313, Title: "Children of Dune", Authors: []string{"Frank Herbert"}, Pages: 408, Slug: "children-of-dune", Rating: 3.85, Ratings: 302},
			{ID: 314, Title: "God Emperor of Dune", Authors: []string{"Frank Herbert"}, Pages: 423, Slug: "god-emperor-of-dune", Rating: 3.80, Ratings: 211},
		},
		recentSearches: []string{"dune", "le guin", "kafka"},
	}
}

// demoLibrary fabricates the two shelves the dashboard opens on, dated
// against now so the pace and the session labels are stable.
func demoLibrary(now time.Time) (reading, oku []model.UserBook) {
	read := func(id, page, startedDaysAgo int) []model.UserBookRead {
		started := now.AddDate(0, 0, -startedDaysAgo)
		return []model.UserBookRead{{ID: id, UserBookID: id, ProgressPages: page, StartedAt: &started}}
	}
	reading = []model.UserBook{
		{
			ID: 101, BookID: 101, StatusID: model.StatusCurrentlyReading, Rating: 4,
			Review:        "Shorter than I expected; the preface is the best part, and the footnotes are where the argument actually lives. Worth the evening it takes.",
			UserBookReads: read(101, 70, 24), UpdatedAt: now,
			Book: model.Book{
				ID: 101, Title: "The Communist Manifesto", Authors: []string{"Karl Marx", "Friedrich Engels"},
				Pages: 305, Slug: "the-communist-manifesto", Rating: 3.60, RatingsCount: 382,
				ReleaseDate: "1848-02-21",
				CachedTags:  `{"Genre":[{"tag":"Politics"},{"tag":"Philosophy"},{"tag":"Classics"},{"tag":"Nonfiction"}]}`,
			},
		},
		{
			ID: 102, BookID: 102, StatusID: model.StatusCurrentlyReading,
			UserBookReads: read(102, 360, 40), UpdatedAt: now,
			Book: model.Book{
				ID: 102, Title: "Money", Authors: []string{"David McWilliams"}, Pages: 416,
				Slug: "money", Rating: 4.02, RatingsCount: 96, ReleaseDate: "2024-09-05",
				CachedTags: `{"Genre":[{"tag":"Economics"},{"tag":"History"}]}`,
			},
		},
		{
			ID: 103, BookID: 103, StatusID: model.StatusCurrentlyReading, UpdatedAt: now,
			Book: model.Book{
				ID: 103, Title: "Software Architecture: The Hard Parts",
				Authors: []string{"Neal Ford", "Mark Richards", "Pramod Sadalage", "Zhamak Dehghani"},
				Pages:   416, Slug: "software-architecture-the-hard-parts", Rating: 4.10, RatingsCount: 1203,
				ReleaseDate: "2021-10-05", FeaturedSeries: "The Hard Parts", FeaturedSeriesPosition: 1,
			},
		},
	}
	oku = []model.UserBook{
		{ID: 201, BookID: 201, StatusID: model.StatusWantToRead, UpdatedAt: now,
			Book: model.Book{ID: 201, Title: "The Dispossessed", Authors: []string{"Ursula K. Le Guin"}, Pages: 387, Slug: "the-dispossessed", Rating: 4.24, RatingsCount: 5400}},
		{ID: 202, BookID: 202, StatusID: model.StatusWantToRead, UpdatedAt: now,
			Book: model.Book{ID: 202, Title: "Meditations", Authors: []string{"Marcus Aurelius"}, Pages: 254, Slug: "meditations", Rating: 4.30, RatingsCount: 9100}},
		{ID: 203, BookID: 203, StatusID: model.StatusWantToRead, UpdatedAt: now,
			Book: model.Book{ID: 203, Title: "The Essential Kafka", Authors: []string{"Franz Kafka"}, Pages: 528, Slug: "the-essential-kafka"}},
	}
	return reading, oku
}

// demoLocalData fabricates stats and sessions around now, so a fixed clock
// gives a fixed page.
func demoLocalData(now time.Time) (*model.ReadingStats, []model.ReadingSession) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	heatmap := make([]model.DayActivity, 0, 26*7)
	for i := 0; i < 26*7; i++ {
		d := today.AddDate(0, 0, -i)
		weekday := (int(d.Weekday()) + 6) % 7 // Mon=0..Sun=6
		base := []int{28, 35, 22, 44, 30, 55, 18}[weekday]
		variation := (i * 7) % 26
		mins := base + variation
		if i%9 == 0 || i%17 == 0 {
			mins = 0
		}
		heatmap = append(heatmap, model.DayActivity{
			Date:    d,
			Minutes: mins,
		})
	}

	weeklyDays := [7]int{36, 52, 28, 64, 41, 73, 34}
	total := 0
	longestIdx := 0
	for i, m := range weeklyDays {
		total += m
		if m > weeklyDays[longestIdx] {
			longestIdx = i
		}
	}
	stats := model.WeeklyStats{
		Days:       weeklyDays,
		Total:      total,
		Sessions:   11,
		LongestDay: longestIdx,
	}

	id := 0
	makeSession := func(daysAgo, startHour, minutes, bookID int, title string) model.ReadingSession {
		id++
		start := today.AddDate(0, 0, -daysAgo).Add(time.Duration(startHour) * time.Hour).Add(12 * time.Minute)
		end := start.Add(time.Duration(minutes) * time.Minute)
		return model.ReadingSession{
			ID:        id,
			BookID:    bookID,
			StartedAt: start,
			EndedAt:   &end,
			BookTitle: title,
		}
	}
	// The book ids match demoLibrary, so a book's detail pane has its own
	// sessions under it.
	sessions := []model.ReadingSession{
		makeSession(0, 20, 48, 101, "The Communist Manifesto"),
		makeSession(1, 19, 36, 101, "The Communist Manifesto"),
		makeSession(2, 21, 62, 101, "The Communist Manifesto"),
		makeSession(3, 18, 29, 102, "Money"),
		makeSession(4, 20, 55, 103, "Software Architecture: The Hard Parts"),
	}

	goalEnd := time.Date(now.Year(), 12, 31, 0, 0, 0, 0, now.Location())
	readingStats := &model.ReadingStats{
		Year: model.YearSummary{
			Year:          now.Year(),
			BooksFinished: 12,
			PagesRead:     3748,
			AvgRating:     3.9,
			RatedCount:    9,
		},
		Goal: &model.Goal{
			ID:       1,
			Metric:   "books",
			Target:   20,
			Progress: 12,
			State:    "active",
			EndDate:  goalEnd,
		},
		Months: [12]int{3, 2, 0, 4, 1, 2, 0, 0, 0, 0, 0, 0},
		Years: []model.LabelCount{
			{Label: "2023", Count: 14}, {Label: "2024", Count: 21},
			{Label: "2025", Count: 18}, {Label: fmt.Sprintf("%d", now.Year()), Count: 12},
		},
		Ratings: [10]int{0, 0, 0, 1, 0, 2, 1, 6, 2, 2},
		Genres: []model.LabelCount{
			{Label: "Fantasy", Count: 8}, {Label: "Classics", Count: 6},
			{Label: "Sci-Fi", Count: 4}, {Label: "Philosophy", Count: 3},
			{Label: "Nonfiction", Count: 2},
		},
		Heatmap: heatmap,
		Weekly:  stats,
	}

	return readingStats, sessions
}
