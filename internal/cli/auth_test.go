package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/auth"
	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

// TestMain keeps every test in this package off the real keychain, even one
// that reaches auth without going through the authBackend fake.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

// stubAuthBackend replaces the auth commands' backend with fakes. A nil
// function fails the test if the command calls it.
func stubAuthBackend(t *testing.T, current func(context.Context) (auth.Credential, error), verify func(context.Context, oauth2.TokenSource) (string, *oauth2.Token, error), logout func(context.Context, func(auth.LogoutReport, string)) (auth.LogoutReport, error)) {
	t.Helper()
	orig := authBackend
	t.Cleanup(func() { authBackend = orig })

	authBackend.current = func(ctx context.Context) (auth.Credential, error) {
		if current == nil {
			t.Fatal("unexpected call to current")
		}
		return current(ctx)
	}
	authBackend.verify = func(ctx context.Context, src oauth2.TokenSource) (string, *oauth2.Token, error) {
		if verify == nil {
			t.Fatal("unexpected call to verify")
		}
		return verify(ctx, src)
	}
	authBackend.logout = func(ctx context.Context, beforeClear func(auth.LogoutReport, string)) (auth.LogoutReport, error) {
		if logout == nil {
			t.Fatal("unexpected call to logout")
		}
		return logout(ctx, beforeClear)
	}
}

// stubLogout fakes a logout that finds report (and legacyKey) before
// clearing the keychain, then fails with err.
func stubLogout(t *testing.T, report auth.LogoutReport, legacyKey string, err error) {
	t.Helper()
	stubAuthBackend(t, nil, nil, func(_ context.Context, beforeClear func(auth.LogoutReport, string)) (auth.LogoutReport, error) {
		beforeClear(report, legacyKey)
		return report, err
	})
}

func runLogout(t *testing.T, stdin string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newLogoutCmd()
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func TestLogoutBrowserLogin(t *testing.T) {
	stubLogout(t, auth.LogoutReport{Kind: auth.KindBrowserLogin}, "", nil)

	out, err := runLogout(t, "")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if out != "Logged out.\n" {
		t.Fatalf("logout output = %q, want only the confirmation", out)
	}
}

func TestLogoutWarnsWhenRevokeFails(t *testing.T) {
	stubLogout(t, auth.LogoutReport{Kind: auth.KindBrowserLogin, RevokeErr: errors.New("revoke token: 500")}, "", nil)

	out, err := runLogout(t, "")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if want := "warning: failed to revoke token with Hardcover: revoke token: 500\nLogged out.\n"; out != want {
		t.Fatalf("logout output = %q, want %q", out, want)
	}
}

func TestLogoutHintsAccountPageForPersonalAccessTokens(t *testing.T) {
	stubLogout(t, auth.LogoutReport{Kind: auth.KindPersonalAPIKey}, "", nil)

	out, err := runLogout(t, "")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if want := "Hint: API keys have to be revoked manually at https://hardcover.app/account/api\nLogged out.\n"; out != want {
		t.Fatalf("logout output = %q, want %q", out, want)
	}
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
			stubLogout(t, auth.LogoutReport{Kind: auth.KindLegacyAPIKey}, jwt, nil)

			out, err := runLogout(t, tt.stdin)
			if err != nil {
				t.Fatalf("logout: %v", err)
			}
			if !strings.Contains(out, "https://api.hardcover.app/invalidate_keys/new") {
				t.Fatalf("logout output has no invalidate URL:\n%s", out)
			}
			if got := strings.Contains(out, jwt); got != tt.showsKey {
				t.Fatalf("key shown = %v, want %v:\n%s", got, tt.showsKey, out)
			}
			if !strings.HasSuffix(out, "Logged out.\n") {
				t.Fatalf("logout output does not confirm the logout:\n%s", out)
			}
		})
	}
}

func TestLogoutWarnsWhenEnvTokenSet(t *testing.T) {
	stubLogout(t, auth.LogoutReport{EnvTokenSet: true}, "", nil)

	out, err := runLogout(t, "")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if !strings.Contains(out, "HARDCOVER_TOKEN is set") {
		t.Fatalf("logout output does not name HARDCOVER_TOKEN:\n%s", out)
	}
}

func TestLogoutClearFailure(t *testing.T) {
	stubLogout(t, auth.LogoutReport{Kind: auth.KindPersonalAPIKey}, "", errors.New("keychain locked"))

	out, err := runLogout(t, "")
	if err == nil || err.Error() != "failed to clear stored token: keychain locked" {
		t.Fatalf("err = %v, want the clear failure", err)
	}
	if !strings.Contains(out, "https://hardcover.app/account/api") {
		t.Fatalf("logout output has no revoke hint:\n%s", out)
	}
	if strings.Contains(out, "Logged out.") {
		t.Fatalf("failed logout reported success:\n%s", out)
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

// stubStatus serves cred as the current login and has Hardcover accept it
// for username, reporting src's token as the one in use.
func stubStatus(t *testing.T, cred auth.Credential, username string) {
	t.Helper()
	stubAuthBackend(t,
		func(context.Context) (auth.Credential, error) { return cred, nil },
		func(_ context.Context, src oauth2.TokenSource) (string, *oauth2.Token, error) {
			token, err := src.Token()
			return username, token, err
		},
		nil)
}

func TestAuthStatusReportsCredential(t *testing.T) {
	expiry := time.Date(2026, 10, 11, 12, 0, 0, 0, time.Local)
	tests := []struct {
		name  string
		kind  auth.Kind
		token *oauth2.Token
		want  []string
	}{
		{
			name:  "browser login",
			kind:  auth.KindBrowserLogin,
			token: &oauth2.Token{AccessToken: "hc_at_abc", RefreshToken: "hc_rt_abc", Expiry: expiry},
			want:  []string{"Using: browser login", "expires 11 Oct 2026 12:00", "renews it automatically"},
		},
		{
			name:  "personal access token",
			kind:  auth.KindPersonalAPIKey,
			token: &oauth2.Token{AccessToken: "hc_pat_abc"},
			want:  []string{"Using: personal API key", "https://hardcover.app/account/api"},
		},
		{
			name:  "legacy JWT",
			kind:  auth.KindLegacyAPIKey,
			token: &oauth2.Token{AccessToken: "eyJhbGciOiJIUzI1NiJ9.e30.sig"},
			want:  []string{"Using: legacy API key", "oku auth login"},
		},
		{
			name:  "environment variable",
			kind:  auth.KindEnv,
			token: &oauth2.Token{AccessToken: "hc_pat_env"},
			want:  []string{"Using: HARDCOVER_TOKEN environment variable"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubStatus(t, auth.Credential{Kind: tt.kind, TokenSource: oauth2.StaticTokenSource(tt.token)}, "reader")

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
	notLoggedIn := errors.New("no token found; run: oku auth login")
	stubAuthBackend(t, func(context.Context) (auth.Credential, error) { return auth.Credential{}, notLoggedIn }, nil, nil)

	_, err := runAuthStatus(t)
	if !errors.Is(err, notLoggedIn) {
		t.Fatalf("err = %v, want the not-logged-in error unchanged", err)
	}
}

func TestAuthStatusRejectedToken(t *testing.T) {
	cred := auth.Credential{Kind: auth.KindPersonalAPIKey, TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "hc_pat_revoked"})}
	stubAuthBackend(t,
		func(context.Context) (auth.Credential, error) { return cred, nil },
		func(context.Context, oauth2.TokenSource) (string, *oauth2.Token, error) {
			return "", nil, api.ErrUnauthorized
		},
		nil)

	out, err := runAuthStatus(t)
	if !errors.Is(err, api.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if strings.Contains(out, "Logged in") {
		t.Fatalf("rejected token reported as logged in:\n%s", out)
	}
}

func TestLogoutOutputOrder(t *testing.T) {
	stubLogout(t, auth.LogoutReport{Kind: auth.KindPersonalAPIKey, RevokeErr: errors.New("boom"), EnvTokenSet: true}, "", nil)

	out, err := runLogout(t, "")
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	want := "warning: HARDCOVER_TOKEN is set; it still overrides the stored login and was not cleared\n" +
		"warning: failed to revoke token with Hardcover: boom\n" +
		"Hint: API keys have to be revoked manually at https://hardcover.app/account/api\n" +
		"Logged out.\n"
	if out != want {
		t.Fatalf("logout output = %q, want %q", out, want)
	}
}
