package app

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/model"
)

// userBookResponse is a user-books response shaped like Hardcover's, with
// every field the extended query requests filled in.
const userBookResponse = `{"me":[{"user_books":[{
	"id": 501,
	"status_id": 3,
	"rating": 4.5,
	"review_raw": "Fear is the mind-killer.\nStill holds up.",
	"reviewed_at": "2025-03-04T20:15:00+00:00",
	"updated_at": "2025-03-05T08:09:10.123456+00:00",
	"user_book_reads": [
		{"id": 9002, "progress_pages": 412, "started_at": "2025-02-01", "finished_at": "2025-03-04"},
		{"id": 9001, "progress_pages": 200, "started_at": "2019-06-01", "finished_at": null}
	],
	"book": {
		"id": 7,
		"title": "Dune",
		"pages": 412,
		"slug": "dune",
		"rating": 4.25,
		"ratings_count": 12000,
		"reviews_count": 900,
		"users_count": 40000,
		"users_read_count": 30000,
		"release_date": "1965-08-01",
		"cached_tags": {"Genre":[{"tag":"Science Fiction"},{"tag":"Classics"}]},
		"featured_book_series": {"position": 1.5, "series": {"id": 55, "name": "Dune"}},
		"contributions": [{"author": {"name": "Frank Herbert"}}, {"author": {"name": "Brian Herbert"}}],
		"image": {"url": "https://assets.hardcover.app/dune.jpg"}
	}
}]}]}`

func day(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

// TestUserBookSurvivesSyncAndCache follows a Hardcover user book through the
// sync path: decode, map, store, read back. Every field the query requests
// must arrive in the model the TUI and CLI render.
func TestUserBookSurvivesSyncAndCache(t *testing.T) {
	var resp api.UserBooksResponse
	if err := json.Unmarshal([]byte(userBookResponse), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	hc := newFakeHardcover()
	hc.userBooks[model.StatusRead] = resp.Me[0].UserBooks
	a := newTestAppWith(t, hc)

	reviewedAt := time.Date(2025, 3, 4, 20, 15, 0, 0, time.UTC)
	want := model.UserBook{
		ID:         501,
		BookID:     7,
		StatusID:   model.StatusRead,
		Rating:     4.5,
		Review:     "Fear is the mind-killer.\nStill holds up.",
		ReviewedAt: &reviewedAt,
		// The cache stores whole seconds.
		UpdatedAt: time.Date(2025, 3, 5, 8, 9, 10, 0, time.UTC),
		Book: model.Book{
			ID:                     7,
			Title:                  "Dune",
			Authors:                []string{"Frank Herbert", "Brian Herbert"},
			Pages:                  412,
			Slug:                   "dune",
			ImageURL:               "https://assets.hardcover.app/dune.jpg",
			Rating:                 4.25,
			RatingsCount:           12000,
			ReviewsCount:           900,
			UsersCount:             40000,
			UsersReadCount:         30000,
			ReleaseDate:            "1965-08-01",
			CachedTags:             `{"Genre":[{"tag":"Science Fiction"},{"tag":"Classics"}]}`,
			FeaturedSeries:         "Dune",
			FeaturedSeriesPosition: 1.5,
		},
		// The cache keeps the latest read only.
		UserBookReads: []model.UserBookRead{
			{ID: 9002, UserBookID: 501, ProgressPages: 412, StartedAt: day(2025, 2, 1), FinishedAt: day(2025, 3, 4)},
		},
	}
	// CurrentPage is derived locally, never sent by Hardcover.
	requireAllFieldsSet(t, want, "CurrentPage")

	books, err := a.ListBooks(context.Background(), model.StatusRead, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("got %d books, want 1", len(books))
	}
	if !reflect.DeepEqual(books[0], want) {
		t.Fatalf("listed book lost fields:\n got %+v\nwant %+v", books[0], want)
	}

	got, err := a.store.GetUserBookByBookID(7)
	if err != nil || got == nil {
		t.Fatalf("GetUserBookByBookID: %v, %v", got, err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("fetched book lost fields:\n got %+v\nwant %+v", *got, want)
	}

	latest, err := a.store.GetLatestRead(501)
	if err != nil || latest == nil || !reflect.DeepEqual(*latest, want.UserBookReads[0]) {
		t.Fatalf("GetLatestRead = %+v, %v; want %+v", latest, err, want.UserBookReads[0])
	}
}

// requireAllFieldsSet fails when a field of v, or of a struct inside it, is
// zero, so a field added to the model without being mapped and cached shows
// up here instead of as a blank in the UI.
func requireAllFieldsSet(t *testing.T, v any, except ...string) {
	t.Helper()
	skip := map[string]bool{}
	for _, name := range except {
		skip[name] = true
	}
	var walk func(prefix string, rv reflect.Value)
	walk = func(prefix string, rv reflect.Value) {
		for i := 0; i < rv.NumField(); i++ {
			f := rv.Type().Field(i)
			if skip[f.Name] {
				continue
			}
			fv := rv.Field(i)
			if fv.IsZero() {
				t.Errorf("fixture leaves %s%s unset", prefix, f.Name)
				continue
			}
			if fv.Kind() == reflect.Struct && f.Type != reflect.TypeOf(time.Time{}) {
				walk(prefix+f.Name+".", fv)
			}
		}
	}
	walk("", reflect.ValueOf(v))
}

func TestMapBookWithoutSeries(t *testing.T) {
	b := mapBook(api.APIBook{ID: 1, Title: "Standalone"})
	if b.FeaturedSeries != "" || b.FeaturedSeriesPosition != 0 {
		t.Fatalf("standalone book got series %q #%v", b.FeaturedSeries, b.FeaturedSeriesPosition)
	}
	unnumbered := &api.APIBookSeries{}
	unnumbered.Series.Name = "Discworld"
	b = mapBook(api.APIBook{FeaturedSeries: unnumbered})
	if b.SeriesLabel() != "Discworld" {
		t.Fatalf("series without a position = %q, want Discworld", b.SeriesLabel())
	}
}

// TestReaderMappingsMatchCLIFixtures checks that the reader commands' data
// maps to the model values the CLI's --json shape tests start from.
func TestReaderMappingsMatchCLIFixtures(t *testing.T) {
	hc := newFakeHardcover()
	if err := json.Unmarshal([]byte(`[
		{"id":1,"goal":20,"metric":"book","progress":12.5,"state":"active","start_date":"2026-01-01","end_date":"2026-12-31"},
		{"id":2,"goal":5000,"metric":"page","progress":"300","state":"completed","start_date":null,"end_date":null}
	]`), &hc.goals); err != nil {
		t.Fatal(err)
	}
	var journal []api.JournalEntry
	if err := json.Unmarshal([]byte(`[
		{"id":41,"book_id":7,"event":"quote","entry":"\"Fear is the mind-killer.\" <b>&","action_at":"2026-03-01"},
		{"id":42,"book_id":7,"event":"note","entry":"Reread.","action_at":"2026-03-02"}
	]`), &journal); err != nil {
		t.Fatal(err)
	}
	hc.bookJournal = map[int][]api.JournalEntry{7: journal}
	var detail api.BookDetail
	if err := json.Unmarshal([]byte(`{"id":7,"title":"Dune","slug":"dune","pages":412,"rating":4.25,"ratings_count":1200,
		"description":"A desert planet.","headline":"Spice","editions_count":3,
		"ratings_distribution":[{"rating":5,"count":10},{"rating":4.5,"count":2}],
		"contributions":[{"author":{"name":"Frank Herbert"}}]}`), &detail); err != nil {
		t.Fatal(err)
	}
	hc.details = map[int]*api.BookDetail{7: &detail}
	a := newTestAppWith(t, hc)
	ctx := context.Background()

	goals, err := a.ListGoals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantGoals := []model.Goal{
		{ID: 1, Metric: "book", Target: 20, Progress: 12.5, State: "active",
			StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
		{ID: 2, Metric: "page", Target: 5000, Progress: 300, State: "completed"},
	}
	if !reflect.DeepEqual(goals, wantGoals) {
		t.Fatalf("goals = %+v, want %+v", goals, wantGoals)
	}

	entries, err := a.BookJournal(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}
	wantEntries := []model.BookJournalEntry{
		{ID: 41, BookID: 7, Event: "quote", Entry: `"Fear is the mind-killer." <b>&`, ActionAt: "2026-03-01"},
		{ID: 42, BookID: 7, Event: "note", Entry: "Reread.", ActionAt: "2026-03-02"},
	}
	if !reflect.DeepEqual(entries, wantEntries) {
		t.Fatalf("journal = %+v, want %+v", entries, wantEntries)
	}

	wantDetail := model.BookDetail{
		Book: model.Book{ID: 7, Title: "Dune", Authors: []string{"Frank Herbert"}, Pages: 412, Slug: "dune",
			Rating: 4.25, RatingsCount: 1200},
		Description:         "A desert planet.",
		Headline:            "Spice",
		EditionsCount:       3,
		RatingsDistribution: `[{"rating":5,"count":10},{"rating":4.5,"count":2}]`,
	}
	// The first call fetches and caches; the second is served from the cache.
	for _, source := range []string{"Hardcover", "cache"} {
		got, err := a.GetBookDetail(ctx, 7, false)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(*got, wantDetail) {
			t.Fatalf("detail from %s = %+v, want %+v", source, *got, wantDetail)
		}
	}
	assertCalls(t, hc, "BookDetail", "BookDetail(7)")
}
