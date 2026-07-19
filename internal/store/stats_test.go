package store

import (
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// seedFinishedRead inserts a book + user_book + finished read in one go.
func seedFinishedRead(t *testing.T, s *Store, id int, pages int, rating float64, finished string, cachedTags string) {
	t.Helper()
	ub := model.UserBook{
		ID:       id,
		BookID:   id,
		StatusID: model.StatusRead,
		Rating:   rating,
		Book: model.Book{
			ID:         id,
			Title:      "Book",
			Pages:      pages,
			CachedTags: cachedTags,
		},
		UpdatedAt: time.Now(),
	}
	if err := s.UpsertUserBook(ub); err != nil {
		t.Fatalf("UpsertUserBook: %v", err)
	}
	if finished != "" {
		ft, err := time.Parse("2006-01-02", finished)
		if err != nil {
			t.Fatalf("parse finished %q: %v", finished, err)
		}
		if err := s.UpsertUserBookRead(model.UserBookRead{
			ID:            id,
			UserBookID:    id,
			ProgressPages: pages,
			FinishedAt:    &ft,
		}); err != nil {
			t.Fatalf("UpsertUserBookRead: %v", err)
		}
	}
}

const genreTagsJSON = `{"Genre":[{"tag":"Fantasy","tagSlug":"fantasy"},{"tag":"Classics","tagSlug":"classics"}],"Mood":[{"tag":"adventurous"}]}`

func TestYearSummaryAndBuckets(t *testing.T) {
	s := testStore(t)

	seedFinishedRead(t, s, 1, 300, 4.0, "2026-01-15", genreTagsJSON)
	seedFinishedRead(t, s, 2, 200, 5.0, "2026-03-02", genreTagsJSON)
	seedFinishedRead(t, s, 3, 400, 0, "2025-12-30", "") // previous year, unrated
	seedFinishedRead(t, s, 4, 0, 3.0, "", "")           // rated but never finished
	seedFinishedRead(t, s, 5, 150, 2.5, "2026-03-20", "")

	sum, err := s.GetYearSummary(2026)
	if err != nil {
		t.Fatal(err)
	}
	if sum.BooksFinished != 3 {
		t.Fatalf("BooksFinished = %d, want 3", sum.BooksFinished)
	}
	if sum.PagesRead != 650 {
		t.Fatalf("PagesRead = %d, want 650", sum.PagesRead)
	}
	// Rated finished-in-2026 books: 4.0, 5.0, 2.5 → avg ≈ 3.83.
	if sum.RatedCount != 3 {
		t.Fatalf("RatedCount = %d, want 3", sum.RatedCount)
	}
	if sum.AvgRating < 3.8 || sum.AvgRating > 3.9 {
		t.Fatalf("AvgRating = %f, want ~3.83", sum.AvgRating)
	}

	months, err := s.GetBooksPerMonth(2026)
	if err != nil {
		t.Fatal(err)
	}
	if months[0] != 1 || months[2] != 2 {
		t.Fatalf("months = %v, want Jan=1 Mar=2", months)
	}

	years, err := s.GetBooksPerYear()
	if err != nil {
		t.Fatal(err)
	}
	want := []model.LabelCount{{Label: "2025", Count: 1}, {Label: "2026", Count: 3}}
	if len(years) != 2 || years[0] != want[0] || years[1] != want[1] {
		t.Fatalf("years = %v, want %v", years, want)
	}
}

func TestRatingsDistributionHalfStarBuckets(t *testing.T) {
	s := testStore(t)

	seedFinishedRead(t, s, 1, 100, 4.5, "2026-01-01", "")
	seedFinishedRead(t, s, 2, 100, 4.5, "2026-01-02", "")
	seedFinishedRead(t, s, 3, 100, 0.5, "2026-01-03", "")
	seedFinishedRead(t, s, 4, 100, 0, "2026-01-04", "") // unrated → excluded

	dist, err := s.GetRatingsDistribution()
	if err != nil {
		t.Fatal(err)
	}
	if dist[8] != 2 { // 4.5 stars = bucket 9 → index 8
		t.Fatalf("dist[8] = %d, want 2", dist[8])
	}
	if dist[0] != 1 { // 0.5 stars
		t.Fatalf("dist[0] = %d, want 1", dist[0])
	}
	total := 0
	for _, n := range dist {
		total += n
	}
	if total != 3 {
		t.Fatalf("total rated = %d, want 3", total)
	}
}

func TestGenreBreakdownCountsFinishedBooksOnly(t *testing.T) {
	s := testStore(t)

	seedFinishedRead(t, s, 1, 100, 0, "2026-01-01", genreTagsJSON)
	seedFinishedRead(t, s, 2, 100, 0, "2026-01-02", genreTagsJSON)

	// A want-to-read book must not count.
	ub := model.UserBook{
		ID: 3, BookID: 3, StatusID: model.StatusWantToRead,
		Book:      model.Book{ID: 3, CachedTags: genreTagsJSON},
		UpdatedAt: time.Now(),
	}
	if err := s.UpsertUserBook(ub); err != nil {
		t.Fatal(err)
	}

	genres, err := s.GetGenreBreakdown(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(genres) != 2 {
		t.Fatalf("genres = %v, want 2 entries", genres)
	}
	for _, g := range genres {
		if g.Count != 2 {
			t.Fatalf("genre %q count = %d, want 2", g.Label, g.Count)
		}
	}
}

func TestJournalReplaceKeepsRecentLocalRows(t *testing.T) {
	s := testStore(t)

	now := time.Now()
	if err := s.InsertLocalJournal(now, "progress_updated"); err != nil {
		t.Fatal(err)
	}

	server := []model.JournalEntry{
		{ID: 100, ActionAt: now.AddDate(0, 0, -2), Event: "progress_updated"},
	}
	if err := s.ReplaceReadingJournals(server); err != nil {
		t.Fatal(err)
	}

	days, err := s.GetJournalDays(now.AddDate(0, 0, -7), now)
	if err != nil {
		t.Fatal(err)
	}
	// The local optimistic row (today) survives the replace alongside the
	// server row (two days ago).
	if len(days) != 2 {
		t.Fatalf("journal days = %d, want 2 (server row + local row)", len(days))
	}

	// A second replace with the same server set must not duplicate anything.
	if err := s.ReplaceReadingJournals(server); err != nil {
		t.Fatal(err)
	}
	days, err = s.GetJournalDays(now.AddDate(0, 0, -7), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 {
		t.Fatalf("journal days after re-sync = %d, want 2", len(days))
	}
}

func TestGoalsRoundTrip(t *testing.T) {
	s := testStore(t)

	start, _ := time.ParseInLocation("2006-01-02", "2026-01-01", time.Local)
	end, _ := time.ParseInLocation("2006-01-02", "2026-12-31", time.Local)
	goals := []model.Goal{
		{ID: 1, Metric: "books", Target: 20, Progress: 12, State: "active", StartDate: start, EndDate: end},
		{ID: 2, Metric: "pages", Target: 5000, Progress: 1500, State: "active"},
	}
	if err := s.ReplaceGoals(goals); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListGoals()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d goals, want 2", len(got))
	}
	var books *model.Goal
	for i := range got {
		if got[i].Metric == "books" {
			books = &got[i]
		}
	}
	if books == nil {
		t.Fatal("books goal missing")
	}
	if books.Target != 20 || books.Progress != 12 || books.State != "active" {
		t.Fatalf("books goal = %+v", *books)
	}
	if !books.EndDate.Equal(end) {
		t.Fatalf("EndDate = %v, want %v", books.EndDate, end)
	}

	// Replace with an empty set clears the cache.
	if err := s.ReplaceGoals(nil); err != nil {
		t.Fatal(err)
	}
	got, err = s.ListGoals()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d goals after clear, want 0", len(got))
	}
}
