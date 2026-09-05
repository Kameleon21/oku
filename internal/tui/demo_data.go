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

func demoLocalData() (*model.ReadingStats, []model.ReadingSession) {
	now := time.Now()
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

	makeSession := func(daysAgo, startHour, minutes, bookID int, title string) model.ReadingSession {
		start := today.AddDate(0, 0, -daysAgo).Add(time.Duration(startHour) * time.Hour).Add(12 * time.Minute)
		end := start.Add(time.Duration(minutes) * time.Minute)
		return model.ReadingSession{
			ID:        daysAgo + 1,
			BookID:    bookID,
			StartedAt: start,
			EndedAt:   &end,
			BookTitle: title,
		}
	}
	sessions := []model.ReadingSession{
		makeSession(0, 20, 48, 101, "The Essential Kafka"),
		makeSession(1, 19, 36, 102, "The Communist Manifesto"),
		makeSession(2, 21, 62, 103, "Meditations"),
		makeSession(3, 18, 29, 104, "Dune"),
		makeSession(4, 20, 55, 105, "Atomic Habits"),
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
