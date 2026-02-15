package api

import (
	"encoding/json"
	"testing"
)

func TestFlexibleIntUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    FlexibleInt
		wantErr bool
	}{
		{name: "number", in: `123`, want: 123},
		{name: "quoted number", in: `"456"`, want: 456},
		{name: "null", in: `null`, want: 0},
		{name: "quoted empty", in: `""`, want: 0},
		{name: "invalid", in: `"abc"`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got FlexibleInt
			err := json.Unmarshal([]byte(tt.in), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("value = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestFlexibleFloatUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    FlexibleFloat
		wantErr bool
	}{
		{name: "number", in: `4.2`, want: 4.2},
		{name: "quoted number", in: `"3.75"`, want: 3.75},
		{name: "null", in: `null`, want: 0},
		{name: "quoted empty", in: `""`, want: 0},
		{name: "invalid", in: `"abc"`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got FlexibleFloat
			err := json.Unmarshal([]byte(tt.in), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("value = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTypesenseResultsUnmarshalWithMixedNumericFormats(t *testing.T) {
	raw := []byte(`{
  "hits": [
    {"document": {"id": "101", "title": "Dune", "author_names": ["Frank Herbert"], "pages": 412, "slug": "dune", "rating": 4.24, "ratings_count": 2000}},
    {"document": {"id": 202, "title": "Foundation", "author_names": ["Isaac Asimov"], "pages": "255", "slug": "foundation", "rating": "4.15", "ratings_count": "1500"}}
  ]
}`)

	var results TypesenseResults
	if err := json.Unmarshal(raw, &results); err != nil {
		t.Fatalf("unmarshal typesense results: %v", err)
	}
	if len(results.Hits) != 2 {
		t.Fatalf("hits len = %d, want 2", len(results.Hits))
	}

	first := results.Hits[0].Document
	if int(first.ID) != 101 || int(first.Pages) != 412 {
		t.Fatalf("first doc parse mismatch: id=%d pages=%d", first.ID, first.Pages)
	}

	second := results.Hits[1].Document
	if int(second.ID) != 202 || int(second.Pages) != 255 {
		t.Fatalf("second doc parse mismatch: id=%d pages=%d", second.ID, second.Pages)
	}
	if float64(first.Rating) <= 0 || int(first.Ratings) != 2000 {
		t.Fatalf("first rating parse mismatch: rating=%v ratings=%d", first.Rating, first.Ratings)
	}
	if float64(second.Rating) <= 0 || int(second.Ratings) != 1500 {
		t.Fatalf("second rating parse mismatch: rating=%v ratings=%d", second.Rating, second.Ratings)
	}
}
