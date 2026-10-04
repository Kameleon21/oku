package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/auth"
	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
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

func runAuthStatus(t *testing.T) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newAuthStatusCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func stubVerifyLogin(t *testing.T, username string, err error) {
	t.Helper()
	orig := verifyLogin
	t.Cleanup(func() { verifyLogin = orig })
	verifyLogin = func(_ context.Context, token *oauth2.Token) (string, *oauth2.Token, error) {
		if err != nil {
			return "", nil, err
		}
		return username, token, nil
	}
}

func TestAuthStatusReportsCredential(t *testing.T) {
	expiry := time.Date(2026, 10, 11, 12, 0, 0, 0, time.Local)
	tests := []struct {
		name  string
		env   string
		token *oauth2.Token
		want  []string
	}{
		{
			name:  "browser login",
			token: &oauth2.Token{AccessToken: "hc_at_abc", RefreshToken: "hc_rt_abc", Expiry: expiry},
			want:  []string{"Using: browser login", "expires 11 Oct 2026 12:00", "renews it automatically"},
		},
		{
			name:  "personal access token",
			token: &oauth2.Token{AccessToken: "hc_pat_abc"},
			want:  []string{"Using: personal API key", "https://hardcover.app/account/api"},
		},
		{
			name:  "legacy JWT",
			token: &oauth2.Token{AccessToken: "eyJhbGciOiJIUzI1NiJ9.e30.sig"},
			want:  []string{"Using: legacy API key", "oku auth login"},
		},
		{
			name: "environment variable",
			env:  "hc_pat_env",
			want: []string{"Using: HARDCOVER_TOKEN environment variable"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			t.Setenv("HARDCOVER_TOKEN", tt.env)
			if tt.token != nil {
				if err := auth.SetToken(tt.token); err != nil {
					t.Fatalf("SetToken: %v", err)
				}
			}
			stubVerifyLogin(t, "reader", nil)

			out, err := runAuthStatus(t)
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			for _, want := range append([]string{"Logged in to Hardcover as reader"}, tt.want...) {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestAuthStatusNotLoggedIn(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HARDCOVER_TOKEN", "")
	stubVerifyLogin(t, "", errors.New("verifyLogin should not be called"))

	_, err := runAuthStatus(t)
	if err == nil || !strings.Contains(err.Error(), "oku auth login") {
		t.Fatalf("err = %v, want a hint to run oku auth login", err)
	}
}

func TestAuthStatusRejectedToken(t *testing.T) {
	keyring.MockInit()
	t.Setenv("HARDCOVER_TOKEN", "")
	if err := auth.SetToken(&oauth2.Token{AccessToken: "hc_pat_revoked"}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	stubVerifyLogin(t, "", api.ErrUnauthorized)

	out, err := runAuthStatus(t)
	if !errors.Is(err, api.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if strings.Contains(out, "Logged in") {
		t.Fatalf("rejected token reported as logged in:\n%s", out)
	}
}
