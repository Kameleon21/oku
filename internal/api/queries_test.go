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
		gql:   graphql.NewClient(srv.URL),
		token: "Bearer test",
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
		gql:   graphql.NewClient(srv.URL),
		token: "Bearer test",
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
