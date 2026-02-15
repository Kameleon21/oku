package api

import "testing"

func TestNormalizeToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "already has Bearer prefix",
			input: "Bearer abc123",
			want:  "Bearer abc123",
		},
		{
			name:  "raw token without prefix",
			input: "abc123",
			want:  "Bearer abc123",
		},
		{
			name:  "lowercase bearer is normalised",
			input: "bearer abc123",
			want:  "Bearer abc123",
		},
		{
			name:  "uppercase BEARER is normalised",
			input: "BEARER abc123",
			want:  "Bearer abc123",
		},
		{
			name:  "mixed case Bearer is normalised",
			input: "bEaReR abc123",
			want:  "Bearer abc123",
		},
		{
			name:  "Bearer without space is treated as raw token",
			input: "BearerNoSpace",
			want:  "Bearer BearerNoSpace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeToken(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeToken(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
