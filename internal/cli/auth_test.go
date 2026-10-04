package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func runLogout(t *testing.T) string {
	t.Helper()
	var out bytes.Buffer
	cmd := newLogoutCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	return out.String()
}

func TestLogoutHintsManualRevokeForAPIKeys(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{name: "legacy JWT", token: "eyJhbGciOiJIUzI1NiJ9.e30.sig"},
		{name: "personal access token", token: "hc_pat_abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			t.Setenv("HARDCOVER_TOKEN", "")
			if err := keyring.Set("oku", "hardcover", tt.token); err != nil {
				t.Fatalf("keyring.Set: %v", err)
			}

			out := runLogout(t)
			if !strings.Contains(out, "https://hardcover.app/account/api") {
				t.Fatalf("logout output has no revoke hint:\n%s", out)
			}
			if _, err := keyring.Get("oku", "hardcover"); err != keyring.ErrNotFound {
				t.Fatalf("legacy entry err = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestLogoutWarnsWhenEnvTokenSet(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HARDCOVER_TOKEN", "env-token")

	if out := runLogout(t); !strings.Contains(out, "HARDCOVER_TOKEN is set") {
		t.Fatalf("logout output does not name HARDCOVER_TOKEN:\n%s", out)
	}
}
