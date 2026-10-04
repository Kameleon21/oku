package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func runLogout(t *testing.T, stdin string) string {
	t.Helper()
	var out bytes.Buffer
	cmd := newLogoutCmd()
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	return out.String()
}

func storeLegacyKey(t *testing.T, key string) {
	t.Helper()
	keyring.MockInit()
	t.Setenv("HARDCOVER_TOKEN", "")
	if err := keyring.Set("oku", "hardcover", key); err != nil {
		t.Fatalf("keyring.Set: %v", err)
	}
}

func assertKeyCleared(t *testing.T) {
	t.Helper()
	if _, err := keyring.Get("oku", "hardcover"); err != keyring.ErrNotFound {
		t.Fatalf("legacy entry err = %v, want ErrNotFound", err)
	}
}

func TestLogoutHintsAccountPageForPersonalAccessTokens(t *testing.T) {
	storeLegacyKey(t, "hc_pat_abc")

	out := runLogout(t, "")
	if !strings.Contains(out, "https://hardcover.app/account/api") {
		t.Fatalf("logout output has no revoke hint:\n%s", out)
	}
	if strings.Contains(out, "hc_pat_abc") {
		t.Fatalf("logout output leaked the key:\n%s", out)
	}
	assertKeyCleared(t)
}

func TestLogoutLegacyJWTRevokeHint(t *testing.T) {
	const jwt = "eyJhbGciOiJIUzI1NiJ9.e30.sig"
	tests := []struct {
		name     string
		stdin    string
		showsKey bool
	}{
		{name: "confirmed", stdin: "y\n", showsKey: true},
		{name: "confirmed long form", stdin: "YES\n", showsKey: true},
		{name: "declined", stdin: "n\n"},
		{name: "default", stdin: "\n"},
		{name: "no input", stdin: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storeLegacyKey(t, jwt)

			out := runLogout(t, tt.stdin)
			if !strings.Contains(out, "https://api.hardcover.app/invalidate_keys/new") {
				t.Fatalf("logout output has no invalidate URL:\n%s", out)
			}
			if got := strings.Contains(out, jwt); got != tt.showsKey {
				t.Fatalf("key shown = %v, want %v:\n%s", got, tt.showsKey, out)
			}
			assertKeyCleared(t)
		})
	}
}

func TestLogoutWarnsWhenEnvTokenSet(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HARDCOVER_TOKEN", "env-token")

	if out := runLogout(t, ""); !strings.Contains(out, "HARDCOVER_TOKEN is set") {
		t.Fatalf("logout output does not name HARDCOVER_TOKEN:\n%s", out)
	}
}
