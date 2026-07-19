package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/machinebox/graphql"
)

func TestListGoals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"me":[{"id":7,"goals":[
			{"id":1,"goal":20,"metric":"books","progress":12,"state":"active","start_date":"2026-01-01","end_date":"2026-12-31"}
		]}]}}`))
	}))
	defer srv.Close()

	c := &Client{gql: graphql.NewClient(srv.URL), token: "Bearer test"}

	goals, err := c.ListGoals(t.Context())
	if err != nil {
		t.Fatalf("ListGoals: %v", err)
	}
	if len(goals) != 1 {
		t.Fatalf("got %d goals, want 1", len(goals))
	}
	g := goals[0]
	if g.Goal != 20 || g.Metric != "books" || float64(g.Progress) != 12 || g.State != "active" {
		t.Fatalf("goal = %+v", g)
	}
	if g.EndDate == nil || *g.EndDate != "2026-12-31" {
		t.Fatalf("end_date = %v, want 2026-12-31", g.EndDate)
	}
}

func TestListGoalsNoUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"me":[]}}`))
	}))
	defer srv.Close()

	c := &Client{gql: graphql.NewClient(srv.URL), token: "Bearer test"}
	if _, err := c.ListGoals(t.Context()); err == nil {
		t.Fatal("ListGoals should error when no user is returned")
	}
}

func TestListReadingJournalsFiltersByUserAndDate(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotQuery = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"reading_journals":[
			{"id":10,"action_at":"2026-07-18","event":"progress_updated"},
			{"id":11,"action_at":"2026-07-19T14:00:00Z","event":"status_read"}
		]}}`))
	}))
	defer srv.Close()

	c := &Client{gql: graphql.NewClient(srv.URL), token: "Bearer test"}

	since := time.Date(2025, 7, 19, 0, 0, 0, 0, time.UTC)
	journals, err := c.ListReadingJournals(t.Context(), 7, since)
	if err != nil {
		t.Fatalf("ListReadingJournals: %v", err)
	}
	if len(journals) != 2 {
		t.Fatalf("got %d journals, want 2", len(journals))
	}
	if journals[0].ID != 10 || journals[0].Event != "progress_updated" {
		t.Fatalf("journal[0] = %+v", journals[0])
	}
	if !strings.Contains(gotQuery, `user_id: { _eq: 7 }`) {
		t.Fatalf("query missing user filter: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, `_gte: \"2025-07-19\"`) {
		t.Fatalf("query missing date filter: %s", gotQuery)
	}
}

func TestUserBooksQueryFetchesAllReadsAndTags(t *testing.T) {
	q := userBooksQuery(3, true)
	if strings.Contains(q, "limit: 1") {
		t.Fatal("extended query must fetch all user_book_reads, not just the latest")
	}
	if !strings.Contains(q, "cached_tags") {
		t.Fatal("extended query missing cached_tags")
	}
	// The minimal fallback stays conservative.
	fallbackQ := userBooksQuery(3, false)
	if strings.Contains(fallbackQ, "cached_tags") {
		t.Fatal("fallback query should not request cached_tags")
	}
}
