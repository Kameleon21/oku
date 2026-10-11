package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

// These pin the --json output of the reader commands to what earlier
// releases printed, when the commands returned Hardcover's wire types
// directly. The inputs are what app's mapper produces from the same wire
// data (see app.TestReaderMappingsMatchCLIFixtures).

func readerJSON(t *testing.T, v any) string {
	t.Helper()
	jsonOutput = true
	t.Cleanup(func() { jsonOutput = false })
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := readerOutput(cmd, v, func() error { t.Fatal("human output used"); return nil }); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestGoalsJSONShape(t *testing.T) {
	goals := []model.Goal{
		{ID: 1, Metric: "book", Target: 20, Progress: 12.5, State: "active",
			StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
		{ID: 2, Metric: "page", Target: 5000, Progress: 300, State: "completed"},
	}
	const want = `[
  {
    "id": 1,
    "goal": 20,
    "metric": "book",
    "progress": 12.5,
    "state": "active",
    "start_date": "2026-01-01",
    "end_date": "2026-12-31"
  },
  {
    "id": 2,
    "goal": 5000,
    "metric": "page",
    "progress": 300,
    "state": "completed",
    "start_date": null,
    "end_date": null
  }
]
`
	if got := readerJSON(t, newGoalsJSON(goals)); got != want {
		t.Fatalf("goals --json changed:\n%s\nwant:\n%s", got, want)
	}
	if got := readerJSON(t, newGoalsJSON(nil)); got != "[]\n" {
		t.Fatalf("no goals printed %q, want []", got)
	}
}

func TestJournalJSONShape(t *testing.T) {
	entries := []model.BookJournalEntry{
		{ID: 41, BookID: 7, Event: "quote", Entry: `"Fear is the mind-killer." <b>&`, ActionAt: "2026-03-01"},
		{ID: 42, BookID: 7, Event: "note", Entry: "Reread.", ActionAt: "2026-03-02"},
	}
	const want = `[
  {
    "id": 41,
    "book_id": 7,
    "event": "quote",
    "entry": "\"Fear is the mind-killer.\" \u003cb\u003e\u0026",
    "action_at": "2026-03-01"
  },
  {
    "id": 42,
    "book_id": 7,
    "event": "note",
    "entry": "Reread.",
    "action_at": "2026-03-02"
  }
]
`
	if got := readerJSON(t, entries); got != want {
		t.Fatalf("journal --json changed:\n%s\nwant:\n%s", got, want)
	}
}

func TestBookDetailJSONShape(t *testing.T) {
	b := model.BookDetail{
		Book: model.Book{ID: 7, Title: "Dune", Authors: []string{"Frank Herbert"}, Pages: 412, Slug: "dune",
			Rating: 4.25, RatingsCount: 1200},
		Description:         "A desert planet.",
		Headline:            "Spice",
		EditionsCount:       3,
		RatingsDistribution: `[{"rating":5,"count":10},{"rating":4.5,"count":2}]`,
	}
	const want = `{
  "id": 7,
  "title": "Dune",
  "pages": 412,
  "slug": "dune",
  "rating": 4.25,
  "ratings_count": 1200,
  "reviews_count": 0,
  "users_count": 0,
  "users_read_count": 0,
  "release_date": null,
  "cached_tags": null,
  "contributions": [
    {
      "author": {
        "name": "Frank Herbert"
      }
    }
  ],
  "image": null,
  "featured_book_series": null,
  "description": "A desert planet.",
  "headline": "Spice",
  "editions_count": 3,
  "ratings_distribution": [
    {
      "rating": 5,
      "count": 10
    },
    {
      "rating": 4.5,
      "count": 2
    }
  ]
}
`
	if got := readerJSON(t, newBookDetailJSON(b)); got != want {
		t.Fatalf("book --json changed:\n%s\nwant:\n%s", got, want)
	}
}
