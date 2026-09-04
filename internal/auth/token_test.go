package auth

import "testing"

func TestNormalizeToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "clean token is unchanged", input: "abc123", want: "abc123"},
		{name: "trailing newline is stripped", input: "abc123\n", want: "abc123"},
		{name: "surrounding whitespace is stripped", input: "  abc123\t\r\n", want: "abc123"},
		{name: "whitespace only becomes empty", input: " \n\t", want: ""},
		{name: "inner whitespace is preserved", input: " Bearer abc123 ", want: "Bearer abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeToken(tt.input); got != tt.want {
				t.Fatalf("normalizeToken(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetTokenTrimsEnvToken(t *testing.T) {
	t.Setenv(envKey, "  env-token\n")

	got, err := GetToken()
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got != "env-token" {
		t.Fatalf("GetToken() = %q, want %q", got, "env-token")
	}
}
