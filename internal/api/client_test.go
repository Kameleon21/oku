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
			name:  "lowercase bearer is not recognised",
			input: "bearer abc123",
			want:  "Bearer bearer abc123",
		},
		{
			name:  "Bearer without space is not recognised",
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
