package model

import (
	"encoding/json"
	"time"
)

// Goal represents a Hardcover reading goal.
type Goal struct {
	ID        int
	Metric    string // "books", "pages", ...
	Target    int
	Progress  float64 // current progress toward Target, maintained server-side
	State     string  // "active", "completed", "failed"
	StartDate time.Time
	EndDate   time.Time
}

// Percent returns goal completion as 0-100, capped at 100.
func (g Goal) Percent() int {
	if g.Target <= 0 {
		return 0
	}
	p := int(g.Progress * 100 / float64(g.Target))
	if p > 100 {
		p = 100
	}
	return p
}

// JournalEntry represents a Hardcover reading journal entry (the activity
// timeline that powers the heatmap).
type JournalEntry struct {
	ID       int
	ActionAt time.Time
	Event    string
}

// JournalDay is a calendar day with at least one journal entry.
type JournalDay struct {
	Date  time.Time
	Count int
}

// YearSummary aggregates reading for a single calendar year.
type YearSummary struct {
	Year          int
	BooksFinished int
	PagesRead     int
	AvgRating     float64 // average of the user's ratings on books finished this year (0 = none rated)
	RatedCount    int
}

// LabelCount is a generic label→count pair used by bar charts
// (books per year, genre breakdown, ...).
type LabelCount struct {
	Label string
	Count int
}

// ReadingStats bundles everything the stats view and `oku stats` display.
type ReadingStats struct {
	Year    YearSummary
	Goal    *Goal        // active "books" goal, nil when none
	Months  [12]int      // books finished per month of Year (Jan=0)
	Years   []LabelCount // books finished per year, ascending
	Ratings [10]int      // count per half-star bucket: index i = (i+1)*0.5 stars
	Genres  []LabelCount // top genres across finished books, descending
	Heatmap []DayActivity
	Weekly  WeeklyStats // timer minutes for the current week
}

// TagsForCategory extracts tag names for one category (e.g. "Genre", "Mood")
// from a Hardcover cached_tags JSON blob. Returns nil when the blob is empty
// or malformed.
func TagsForCategory(cachedTags string, category string) []string {
	if cachedTags == "" {
		return nil
	}
	var byCategory map[string][]struct {
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal([]byte(cachedTags), &byCategory); err != nil {
		return nil
	}
	tags := byCategory[category]
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if t.Tag != "" {
			out = append(out, t.Tag)
		}
	}
	return out
}
