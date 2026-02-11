package app

import (
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/model"
)

func TestParseAPITimeSupportsCommonLayouts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		{name: "rfc3339", in: "2025-02-03T04:05:06Z", ok: true},
		{name: "rfc3339nano", in: "2025-02-03T04:05:06.123456789Z", ok: true},
		{name: "date-only", in: "2025-02-03", ok: true},
		{name: "invalid", in: "not-a-time", ok: false},
		{name: "empty", in: "   ", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := parseAPITime(tt.in)
			if ok != tt.ok {
				t.Fatalf("parseAPITime(%q) ok=%v, want %v", tt.in, ok, tt.ok)
			}
		})
	}
}

func TestConvertAPIUserBookMapsExtendedFields(t *testing.T) {
	releaseDate := " 2024-01-05 "
	updatedAt := "2025-02-03T04:05:06Z"
	startedAt := "2025-01-01"
	finishedAt := "2025-01-09T12:00:00Z"

	contrib := api.APIContribution{}
	contrib.Author.Name = "Neal Ford"

	ab := api.APIUserBook{
		ID:        77,
		StatusID:  int(model.StatusCurrentlyReading),
		UpdatedAt: &updatedAt,
		Book: api.APIBook{
			ID:             99,
			Title:          "Software Architecture",
			Pages:          450,
			Slug:           "software-architecture",
			Rating:         4.25,
			RatingsCount:   1234,
			ReviewsCount:   321,
			UsersCount:     4567,
			UsersReadCount: 3456,
			ReleaseDate:    &releaseDate,
			Contributions:  []api.APIContribution{contrib},
			Image:          &api.APIImage{URL: "https://example.com/book.jpg"},
		},
		UserBookReads: []api.APIUserBookRead{
			{
				ID:            15,
				ProgressPages: 88,
				StartedAt:     &startedAt,
				FinishedAt:    &finishedAt,
			},
		},
	}

	got := convertAPIUserBook(ab)
	if got.ID != 77 || got.BookID != 99 {
		t.Fatalf("ids = userBook:%d book:%d, want 77/99", got.ID, got.BookID)
	}
	if got.StatusID != model.StatusCurrentlyReading {
		t.Fatalf("status = %v, want currently reading", got.StatusID)
	}
	if got.Book.Title != "Software Architecture" || got.Book.Slug != "software-architecture" {
		t.Fatalf("book title/slug not mapped: %+v", got.Book)
	}
	if got.Book.ImageURL != "https://example.com/book.jpg" {
		t.Fatalf("image url = %q", got.Book.ImageURL)
	}
	if got.Book.ReleaseDate != "2024-01-05" {
		t.Fatalf("release date = %q, want trimmed value", got.Book.ReleaseDate)
	}
	if len(got.Book.Authors) != 1 || got.Book.Authors[0] != "Neal Ford" {
		t.Fatalf("authors = %#v, want [Neal Ford]", got.Book.Authors)
	}
	if got.Book.Rating != 4.25 || got.Book.RatingsCount != 1234 || got.Book.ReviewsCount != 321 {
		t.Fatalf("book metrics not mapped: %+v", got.Book)
	}
	if got.Book.UsersCount != 4567 || got.Book.UsersReadCount != 3456 {
		t.Fatalf("reader counts not mapped: users=%d read=%d", got.Book.UsersCount, got.Book.UsersReadCount)
	}

	wantUpdated, _ := time.Parse(time.RFC3339, updatedAt)
	if !got.UpdatedAt.Equal(wantUpdated.UTC()) {
		t.Fatalf("updated_at = %s, want %s", got.UpdatedAt, wantUpdated.UTC())
	}
	if len(got.UserBookReads) != 1 {
		t.Fatalf("read entries = %d, want 1", len(got.UserBookReads))
	}
	read := got.UserBookReads[0]
	if read.UserBookID != 77 || read.ProgressPages != 88 {
		t.Fatalf("read mapping mismatch: %+v", read)
	}
	if read.StartedAt == nil || read.FinishedAt == nil {
		t.Fatalf("expected parsed started/finished times, got %+v", read)
	}
}

func TestConvertAPIUserBookFallsBackWhenTimesInvalid(t *testing.T) {
	invalidUpdatedAt := "not-a-time"
	invalidStartedAt := "bad"

	before := time.Now().UTC()
	ab := api.APIUserBook{
		ID:        1,
		StatusID:  int(model.StatusWantToRead),
		UpdatedAt: &invalidUpdatedAt,
		Book: api.APIBook{
			ID:    2,
			Title: "Book",
		},
		UserBookReads: []api.APIUserBookRead{
			{ID: 3, ProgressPages: 10, StartedAt: &invalidStartedAt},
		},
	}
	got := convertAPIUserBook(ab)
	after := time.Now().UTC()

	if got.UpdatedAt.Before(before.Add(-time.Second)) || got.UpdatedAt.After(after.Add(time.Second)) {
		t.Fatalf("updated_at fallback out of range: %s (before=%s after=%s)", got.UpdatedAt, before, after)
	}
	if len(got.UserBookReads) != 1 {
		t.Fatalf("read entries = %d, want 1", len(got.UserBookReads))
	}
	if got.UserBookReads[0].StartedAt != nil {
		t.Fatalf("started_at should stay nil on parse failure, got %v", got.UserBookReads[0].StartedAt)
	}
}
