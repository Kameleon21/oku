package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/auth"
	"github.com/Kameleon21/oku/internal/config"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/store"
	"github.com/spf13/cobra"
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
func stubAuthBackend(t *testing.T, current func(context.Context) (auth.Credential, error), verify func(context.Context, oauth2.TokenSource) (int, string, *oauth2.Token, error), logout func(context.Context, func(auth.LogoutReport, string)) (auth.LogoutReport, error)) {
	t.Helper()
	orig := authBackend
	t.Cleanup(func() { authBackend = orig })

	// The browser sign-in and the token prompt fail the test unless it sets
	// them; recording the account does nothing unless it does.
	authBackend.login = func(context.Context, io.Writer) (*oauth2.Token, error) {
		t.Fatal("unexpected call to login")
		return nil, nil
	}
	authBackend.promptToken = func() (string, error) {
		t.Fatal("unexpected call to promptToken")
		return "", nil
	}
	authBackend.remember = func(int) (bool, error) { return false, nil }
	// Every permission is granted unless the test says otherwise.
	authBackend.checkScopes = func(context.Context, oauth2.TokenSource) ([]string, error) { return nil, nil }

	authBackend.current = func(ctx context.Context) (auth.Credential, error) {
		if current == nil {
			t.Fatal("unexpected call to current")
		}
		return current(ctx)
	}
	authBackend.verify = func(ctx context.Context, src oauth2.TokenSource) (int, string, *oauth2.Token, error) {
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
		func(_ context.Context, src oauth2.TokenSource) (int, string, *oauth2.Token, error) {
			token, err := src.Token()
			return 1, username, token, err
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
		func(context.Context, oauth2.TokenSource) (int, string, *oauth2.Token, error) {
			return 0, "", nil, api.ErrUnauthorized
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

// TestLoginAsAnotherAccountClearsTheCache: signing in as B on a machine that
// holds A's cached library drops A's books, so B never sees them.
func TestLoginAsAnotherAccountClearsTheCache(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HARDCOVER_TOKEN", "")

	dbPath, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := config.EnsureDataDir(); err != nil {
		t.Fatal(err)
	}
	db, err := store.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertUserBook(model.UserBook{ID: 1, BookID: 11, StatusID: model.StatusCurrentlyReading,
		Book: model.Book{ID: 11, Title: "Dune"}, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetState("user_id", "1"); err != nil {
		t.Fatal(err)
	}
	db.Close()

	tokenB := &oauth2.Token{AccessToken: "hc_at_b", TokenType: "Bearer"}
	stubAuthBackend(t,
		func(context.Context) (auth.Credential, error) {
			return auth.Credential{Kind: auth.KindBrowserLogin, TokenSource: oauth2.StaticTokenSource(tokenB)}, nil
		},
		func(_ context.Context, src oauth2.TokenSource) (int, string, *oauth2.Token, error) {
			token, err := src.Token()
			return 2, "b", token, err
		},
		nil)
	authBackend.login = func(context.Context, io.Writer) (*oauth2.Token, error) { return tokenB, nil }
	authBackend.remember = rememberSignedInUser

	var out bytes.Buffer
	cmd := newLoginCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("login: %v", err)
	}
	if want := "Login successful.\nSigned in as a different account; cleared the cached library. Run oku sync.\n"; out.String() != want {
		t.Fatalf("login output = %q, want %q", out.String(), want)
	}

	db, err = store.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if books, _ := db.ListUserBooks(model.StatusCurrentlyReading); len(books) != 0 {
		t.Fatalf("cached reading = %+v, want A's books gone", books)
	}
	if v, _ := db.GetState("user_id"); v != "2" {
		t.Fatalf("user_id = %q, want B", v)
	}
}

func TestAuthStatusNotesAClearedCache(t *testing.T) {
	cred := auth.Credential{Kind: auth.KindPersonalAPIKey, TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "hc_pat_b"})}
	for name, tt := range map[string]struct {
		cleared bool
		err     error
		want    bool
	}{
		"cleared":      {cleared: true, want: true},
		"same account": {},
		"no data dir":  {err: errors.New("create data dir: read-only file system")},
	} {
		t.Run(name, func(t *testing.T) {
			stubStatus(t, cred, "b")
			authBackend.remember = func(int) (bool, error) { return tt.cleared, tt.err }

			var stdout, stderr bytes.Buffer
			cmd := newAuthStatusCmd()
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("status: %v", err)
			}
			if got := strings.Contains(stderr.String(), "cleared the cached library"); got != tt.want || (!tt.want && stderr.Len() != 0) {
				t.Fatalf("stderr = %q, want the note: %v", stderr.String(), tt.want)
			}
			if !strings.Contains(stdout.String(), "Logged in to Hardcover as b") {
				t.Fatalf("stdout = %q, want the status", stdout.String())
			}
		})
	}
}

func TestAuthStatusListsMissingPermissions(t *testing.T) {
	tests := []struct {
		kind auth.Kind
		want string
	}{
		{auth.KindBrowserLogin, "Run `oku auth login` again and approve them"},
		{auth.KindPersonalAPIKey, "oku auth set-token"},
		{auth.KindEnv, "HARDCOVER_TOKEN is in use"},
	}
	for _, tt := range tests {
		t.Run(tt.kind.String(), func(t *testing.T) {
			stubStatus(t, auth.Credential{Kind: tt.kind, TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "x"})}, "reader")
			authBackend.checkScopes = func(context.Context, oauth2.TokenSource) ([]string, error) {
				return []string{"read:journal", "read:goals"}, nil
			}

			out, err := runAuthStatus(t)
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			for _, want := range []string{"Missing permissions:", "read:journal  notes, quotes and the activity heatmap", "read:goals    reading goals", tt.want} {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestAuthStatusPermissionsGranted(t *testing.T) {
	stubStatus(t, auth.Credential{Kind: auth.KindBrowserLogin, TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "x"})}, "reader")
	out, err := runAuthStatus(t)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "Permissions: every read permission granted") || strings.Contains(out, "Missing") {
		t.Fatalf("output = %q", out)
	}
}

func TestAuthStatusPermissionCheckFailureIsOnlyAWarning(t *testing.T) {
	stubStatus(t, auth.Credential{Kind: auth.KindBrowserLogin, TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "x"})}, "reader")
	authBackend.checkScopes = func(context.Context, oauth2.TokenSource) ([]string, error) {
		return nil, errors.New("network down")
	}
	out, err := runAuthStatus(t)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "warning: could not check permissions: network down") {
		t.Fatalf("output = %q", out)
	}
}

// runSignIn runs a sign-in command (login or set-token) that ends up with
// token, Hardcover accepting it, and returns everything printed. probe, when
// set, answers the permission check; otherwise every read permission works.
func runSignIn(t *testing.T, cmd func() *cobra.Command, token *oauth2.Token, probe func(context.Context, oauth2.TokenSource) ([]string, error)) string {
	t.Helper()
	stubAuthBackend(t,
		func(context.Context) (auth.Credential, error) {
			return auth.Credential{Kind: auth.KindBrowserLogin, TokenSource: oauth2.StaticTokenSource(token)}, nil
		},
		func(_ context.Context, src oauth2.TokenSource) (int, string, *oauth2.Token, error) {
			tok, err := src.Token()
			return 1, "reader", tok, err
		},
		nil)
	authBackend.login = func(context.Context, io.Writer) (*oauth2.Token, error) { return token, nil }
	authBackend.promptToken = func() (string, error) { return token.AccessToken, nil }
	if probe != nil {
		authBackend.checkScopes = probe
	}

	var out bytes.Buffer
	c := cmd()
	c.SetOut(&out)
	c.SetErr(&out)
	if err := c.Execute(); err != nil {
		t.Fatalf("%s: %v", c.Name(), err)
	}
	return out.String()
}

func TestLoginWarnsAboutUngrantedScopes(t *testing.T) {
	// The consent screen from issue #59 dropped three read permissions.
	token := (&oauth2.Token{AccessToken: "hc_at_x", RefreshToken: "hc_rt_x"}).WithExtra(map[string]any{
		"scope": "read:me:content read:library read:catalog write:library write:reviews write:goals write:lists",
	})
	out := runSignIn(t, newLoginCmd, token, nil)
	for _, want := range []string{
		"Login successful.",
		"warning: Hardcover didn't grant oku every permission it needs:",
		"read:journal  notes, quotes and the activity heatmap",
		"read:goals    reading goals",
		"read:lists    the reading queue",
		"Run `oku auth login` again and approve them",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestLoginWithEveryScopeIsQuiet(t *testing.T) {
	token := (&oauth2.Token{AccessToken: "hc_at_x", RefreshToken: "hc_rt_x"}).WithExtra(map[string]any{
		"scope": strings.Join(api.ScopeNames(), " "),
	})
	if out := runSignIn(t, newLoginCmd, token, nil); out != "Login successful.\n" {
		t.Fatalf("output = %q", out)
	}
}

// The token response alone can't be trusted (a server may echo the request),
// so login also asks Hardcover, and reports what either found.
func TestLoginCombinesTheProbeAndTheTokenResponse(t *testing.T) {
	granted := strings.ReplaceAll(strings.Join(api.ScopeNames(), " "), "write:goals", "")
	token := (&oauth2.Token{AccessToken: "hc_at_x", RefreshToken: "hc_rt_x"}).WithExtra(map[string]any{"scope": granted})
	out := runSignIn(t, newLoginCmd, token, func(context.Context, oauth2.TokenSource) ([]string, error) {
		return []string{"read:journal"}, nil
	})
	for _, want := range []string{"read:journal  notes, quotes", "write:goals   editing reading goals"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestLoginProbesWhenScopesAreNotReported(t *testing.T) {
	token := &oauth2.Token{AccessToken: "hc_at_x", RefreshToken: "hc_rt_x"}
	out := runSignIn(t, newLoginCmd, token, func(context.Context, oauth2.TokenSource) ([]string, error) {
		return []string{"read:journal"}, nil
	})
	if !strings.Contains(out, "read:journal  notes, quotes") {
		t.Fatalf("output = %q", out)
	}
}

func TestSetTokenWarnsAboutMissingScopes(t *testing.T) {
	token := &oauth2.Token{AccessToken: "hc_pat_x", TokenType: "Bearer"}
	out := runSignIn(t, newSetTokenCmd, token, func(context.Context, oauth2.TokenSource) ([]string, error) {
		return []string{"read:goals"}, nil
	})
	for _, want := range []string{"Token stored successfully.", "read:goals  reading goals", "oku auth set-token", "account/api/keys/new"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestErrorHintForMissingScope(t *testing.T) {
	err := fmt.Errorf("goals: %w", &api.StatusError{Code: 403, APIError: "insufficient_scope", Scope: "read:goals"})

	t.Setenv("HARDCOVER_TOKEN", "")
	if hint := errorHint(err); !strings.Contains(hint, "oku auth login") {
		t.Fatalf("hint = %q, want the login remedy", hint)
	}
	t.Setenv("HARDCOVER_TOKEN", "hc_pat_env")
	if hint := errorHint(err); !strings.Contains(hint, "HARDCOVER_TOKEN is in use") {
		t.Fatalf("hint = %q, want the environment variable remedy", hint)
	}
}

func TestAuthStatusKeepsPermissionsFoundBeforeACheckFails(t *testing.T) {
	stubStatus(t, auth.Credential{Kind: auth.KindBrowserLogin, TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "x"})}, "reader")
	authBackend.checkScopes = func(context.Context, oauth2.TokenSource) ([]string, error) {
		return []string{"read:journal"}, errors.New("timeout")
	}
	out, err := runAuthStatus(t)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, want := range []string{"warning: could not check permissions: timeout", "Missing permissions:", "read:journal"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "every read permission granted") {
		t.Errorf("a failed check must not claim everything is granted:\n%s", out)
	}
}
