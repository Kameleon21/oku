package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/machinebox/graphql"
)

func TestUpdateUserBookReviewMutation(t *testing.T) {
	tests := []struct {
		name              string
		includeReviewedAt bool
		wantContains      []string
		wantNotContains   []string
	}{
		{
			name:              "review_slate with reviewed_at",
			includeReviewedAt: true,
			wantContains: []string{
				"$reviewedAt: date!",
				"$reviewSlate: jsonb!",
				"review_slate: $reviewSlate",
				"reviewed_at: $reviewedAt",
			},
			wantNotContains: []string{
				"review_raw:",
				"review: $review",
			},
		},
		{
			name:              "review_slate without reviewed_at",
			includeReviewedAt: false,
			wantContains: []string{
				"$reviewSlate: jsonb!",
				"review_slate: $reviewSlate",
			},
			wantNotContains: []string{
				"$reviewedAt: date!",
				"reviewed_at: $reviewedAt",
				"review_raw:",
				"review: $review",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := updateUserBookReviewMutation(tt.includeReviewedAt)
			for _, needle := range tt.wantContains {
				if !strings.Contains(got, needle) {
					t.Fatalf("mutation missing %q\n%s", needle, got)
				}
			}
			for _, needle := range tt.wantNotContains {
				if strings.Contains(got, needle) {
					t.Fatalf("mutation should not contain %q\n%s", needle, got)
				}
			}
		})
	}
}

func TestReviewTextToSlate(t *testing.T) {
	got := reviewTextToSlate("First paragraph.\nSecond paragraph.")
	want := []map[string]any{
		{
			"type": "paragraph",
			"children": []map[string]any{
				{"text": "First paragraph."},
			},
		},
		{
			"type": "paragraph",
			"children": []map[string]any{
				{"text": "Second paragraph."},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reviewTextToSlate() = %#v, want %#v", got, want)
	}
}

func TestProgressJournalObject(t *testing.T) {
	got := progressJournalObject(42, 120, 300, 2, "2026-07-19")
	want := map[string]any{
		"book_id":            42,
		"event":              "progress_updated",
		"action_at":          "2026-07-19",
		"privacy_setting_id": 2,
		"tags":               []any{},
		"metadata": map[string]any{
			"position": map[string]any{
				"type":     "pages",
				"value":    120,
				"possible": 300,
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("progressJournalObject() = %#v, want %#v", got, want)
	}
}

func TestProgressJournalObjectOmitsUnknownTotalPages(t *testing.T) {
	got := progressJournalObject(42, 120, 0, 1, "2026-07-19")
	position, ok := got["metadata"].(map[string]any)["position"].(map[string]any)
	if !ok {
		t.Fatalf("metadata.position missing: %#v", got)
	}
	if _, exists := position["possible"]; exists {
		t.Fatalf("position should omit possible when total pages unknown: %#v", position)
	}
}

func TestInsertProgressJournalSendsJournalMutation(t *testing.T) {
	var gotBody struct {
		Query     string `json:"query"`
		Variables struct {
			Object map[string]any `json:"object"`
		} `json:"variables"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"insert_reading_journal":{"reading_journal":{"id":99}}}}`))
	}))
	defer srv.Close()

	c := &Client{
		gql:   graphql.NewClient(srv.URL),
		token: "Bearer test",
	}

	if err := c.InsertProgressJournal(t.Context(), 42, 120, 300, 1); err != nil {
		t.Fatalf("InsertProgressJournal returned error: %v", err)
	}
	if !strings.Contains(gotBody.Query, "insert_reading_journal") {
		t.Fatalf("query should call insert_reading_journal, got: %s", gotBody.Query)
	}
	obj := gotBody.Variables.Object
	if obj["event"] != "progress_updated" {
		t.Fatalf("event = %v, want progress_updated", obj["event"])
	}
	if _, err := time.Parse("2006-01-02", obj["action_at"].(string)); err != nil {
		t.Fatalf("action_at %v is not a date: %v", obj["action_at"], err)
	}
}

func TestInsertProgressJournalErrorsWithoutJournal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"insert_reading_journal":{"reading_journal":null}}}`))
	}))
	defer srv.Close()

	c := &Client{
		gql:   graphql.NewClient(srv.URL),
		token: "Bearer test",
	}

	if err := c.InsertProgressJournal(t.Context(), 42, 120, 300, 1); err == nil {
		t.Fatal("InsertProgressJournal should error when no reading_journal is returned")
	}
}
