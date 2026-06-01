package api

import (
	"reflect"
	"strings"
	"testing"
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
