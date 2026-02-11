package api

import (
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
			wantFields:  `, fields: "title,author_names"`,
			wantWeights: `, weights: "7,3"`,
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
			wantFields:  `, fields: "title,author_names"`,
			wantWeights: `, weights: "7,3"`,
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
