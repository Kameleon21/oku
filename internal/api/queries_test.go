package api

import (
	"strings"
	"testing"

	"github.com/Kameleon21/oku/internal/model"
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
