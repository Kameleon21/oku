package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/machinebox/graphql"
)

func TestSearchConfigForMode(t *testing.T) {
	tests := []struct {
		name        string
		mode        model.SearchMode
		wantFields  string
		wantWeights string
	}{
		{
			name:        "book default",
			mode:        model.SearchModeBook,
			wantFields:  "",
			wantWeights: "",
		},
		{
			name:        "author",
			mode:        model.SearchModeAuthor,
			wantFields:  `, fields: "author_names,title"`,
			wantWeights: `, weights: "8,2"`,
		},
		{
			name:        "genre",
			mode:        model.SearchModeGenre,
			wantFields:  `, fields: "genres,tags,title,author_names"`,
			wantWeights: `, weights: "8,5,2,1"`,
		},
		{
			name:        "unknown falls back to book",
			mode:        model.SearchMode("x"),
			wantFields:  "",
			wantWeights: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := searchConfigForMode(tt.mode)
			if got.fieldsArg != tt.wantFields || got.weightsArg != tt.wantWeights {
				t.Fatalf("searchConfigForMode(%q) = %+v, want fields=%q weights=%q",
					tt.mode, got, tt.wantFields, tt.wantWeights)
			}
		})
	}
}

func TestUserBooksQueryIncludesReviewFieldsInExtendedMode(t *testing.T) {
	q := userBooksQuery(2, true)
	for _, field := range []string{"rating", "review_raw", "reviewed_at", "updated_at"} {
		if !strings.Contains(q, field) {
			t.Fatalf("extended query missing field %q", field)
		}
	}
}

func TestUserBooksQueryOmitsReviewFieldsInFallbackMode(t *testing.T) {
	q := userBooksQuery(2, false)
	for _, field := range []string{"review_raw", "reviewed_at"} {
		if strings.Contains(q, field) {
			t.Fatalf("fallback query should not contain %q", field)
		}
	}
}

func TestSearchBooksUsesDefaultBookSearch(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		gotQuery = body.Query
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"search":{"results":{"hits":[{"document":{"id":"101","title":"The Burning God","author_names":["R. F. Kuang"],"pages":640,"slug":"the-burning-god","rating":4.18,"ratings_count":12000}}]}}}}`))
	}))
	defer srv.Close()

	c := &Client{
		gql: graphql.NewClient(srv.URL),
	}

	results, err := c.SearchBooks(t.Context(), "burning god", 10, model.SearchModeBook)
	if err != nil {
		t.Fatalf("SearchBooks returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].ID != 101 || results[0].Title != "The Burning God" {
		t.Fatalf("unexpected result: %+v", results[0])
	}
	if strings.Contains(gotQuery, "query_type") {
		t.Fatalf("book search should use Hardcover's default query_type, got: %s", gotQuery)
	}
}

func TestSearchBooksFallsBackWhenWeightedSearchIsRejected(t *testing.T) {
	var requests []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		requests = append(requests, body.Query)
		w.Header().Set("Content-Type", "application/json")

		if len(requests) == 1 {
			_, _ = w.Write([]byte(`{"errors":[{"message":"field weights not found"}]}`))
			return
		}

		_, _ = w.Write([]byte(`{"data":{"search":{"results":{"hits":[{"document":{"id":"101","title":"Dune","author_names":["Frank Herbert"],"pages":412,"slug":"dune","rating":4.24,"ratings_count":2000}}]}}}}`))
	}))
	defer srv.Close()

	c := &Client{
		gql: graphql.NewClient(srv.URL),
	}

	results, err := c.SearchBooks(t.Context(), "dune", 10, model.SearchModeAuthor)
	if err != nil {
		t.Fatalf("SearchBooks returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].ID != 101 || results[0].Title != "Dune" {
		t.Fatalf("unexpected result: %+v", results[0])
	}
	if len(requests) != 2 {
		t.Fatalf("requests len = %d, want 2", len(requests))
	}
	if !strings.Contains(requests[0], `fields: "author_names,title"`) {
		t.Fatalf("first request should include weighted fields, got: %s", requests[0])
	}
	if strings.Contains(requests[1], "fields") || strings.Contains(requests[1], "weights") {
		t.Fatalf("fallback request should omit weighted fields, got: %s", requests[1])
	}
}

func TestGetAccountPrivacySetting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"me":[{"id":7,"account_privacy_setting_id":3}]}}`))
	}))
	defer srv.Close()

	c := &Client{
		gql: graphql.NewClient(srv.URL),
	}

	got, err := c.GetAccountPrivacySetting(t.Context())
	if err != nil {
		t.Fatalf("GetAccountPrivacySetting returned error: %v", err)
	}
	if got != 3 {
		t.Fatalf("GetAccountPrivacySetting = %d, want 3", got)
	}
}

func TestGetAccountPrivacySettingMissingField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"me":[{"id":7}]}}`))
	}))
	defer srv.Close()

	c := &Client{
		gql: graphql.NewClient(srv.URL),
	}

	if _, err := c.GetAccountPrivacySetting(t.Context()); err == nil {
		t.Fatal("GetAccountPrivacySetting should error when the field is missing")
	}
}

func TestListUserBooksRequestsAndDecodesFeaturedSeries(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		gotQuery = body.Query
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"me":[{"user_books":[
			{"id":1,"status_id":3,"book":{"id":10,"title":"Dune","featured_book_series":{"position":1,"series":{"id":5,"name":"Dune"}}}},
			{"id":2,"status_id":3,"book":{"id":11,"title":"Gods of Risk","featured_book_series":{"position":2.5,"series":{"id":6,"name":"The Expanse"}}}},
			{"id":3,"status_id":3,"book":{"id":12,"title":"Standalone","featured_book_series":null}}
		]}]}}`))
	}))
	defer srv.Close()

	c := &Client{gql: graphql.NewClient(srv.URL), token: "Bearer test"}
	books, err := c.ListUserBooks(t.Context(), 3)
	if err != nil {
		t.Fatalf("ListUserBooks: %v", err)
	}

	if want := "featured_book_series { position series { id name } }"; !strings.Contains(strings.Join(strings.Fields(gotQuery), " "), want) {
		t.Fatalf("query does not request %q:\n%s", want, gotQuery)
	}
	if len(books) != 3 {
		t.Fatalf("got %d books, want 3", len(books))
	}
	for i, want := range []struct {
		name     string
		position float64
	}{{"Dune", 1}, {"The Expanse", 2.5}} {
		s := books[i].Book.FeaturedSeries
		if s == nil || s.Series.Name != want.name || s.Position == nil || *s.Position != want.position {
			t.Fatalf("book %d series = %+v, want %s #%v", i, s, want.name, want.position)
		}
	}
	if books[2].Book.FeaturedSeries != nil {
		t.Fatalf("standalone book has series %+v", books[2].Book.FeaturedSeries)
	}
}
