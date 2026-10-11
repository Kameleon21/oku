package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

const legacyJWT = "eyJhbGciOiJIUzI1NiJ9.e30.sig"

func TestCurrentPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		env        string
		stored     *oauth2.Token
		legacy     string // plain key in the pre-OAuth keychain entry
		wantKind   Kind
		wantAccess string
	}{
		{name: "browser login", stored: validToken("oauth", "refresh"), wantKind: KindBrowserLogin, wantAccess: "oauth"},
		{name: "personal API key", stored: &oauth2.Token{AccessToken: "hc_pat_abc"}, wantKind: KindPersonalAPIKey, wantAccess: "hc_pat_abc"},
		{name: "legacy JWT", stored: &oauth2.Token{AccessToken: legacyJWT}, wantKind: KindLegacyAPIKey, wantAccess: legacyJWT},
		{name: "legacy keychain entry", legacy: legacyJWT + "\n", wantKind: KindLegacyAPIKey, wantAccess: legacyJWT},
		{name: "env only", env: "env-token", wantKind: KindEnv, wantAccess: "env-token"},
		{name: "env trimmed", env: "  env-token\n", wantKind: KindEnv, wantAccess: "env-token"},
		{name: "env bearer prefix stripped", env: "Bearer env-token\n", wantKind: KindEnv, wantAccess: "env-token"},
		{name: "env over browser login", env: "env-token", stored: validToken("oauth", "refresh"), wantKind: KindEnv, wantAccess: "env-token"},
		{name: "env over personal API key", env: "env-token", stored: &oauth2.Token{AccessToken: "hc_pat_abc"}, wantKind: KindEnv, wantAccess: "env-token"},
		{name: "env over legacy JWT", env: "env-token", stored: &oauth2.Token{AccessToken: legacyJWT}, wantKind: KindEnv, wantAccess: "env-token"},
		{name: "blank env falls back to keychain", env: " \n", stored: &oauth2.Token{AccessToken: "hc_pat_abc"}, wantKind: KindPersonalAPIKey, wantAccess: "hc_pat_abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			freshKeychain(t)
			t.Setenv(envKey, tt.env)
			if tt.stored != nil {
				mustStore(t, tt.stored)
			}
			if tt.legacy != "" {
				if err := keyring.Set(serviceName, legacyAccountName, tt.legacy); err != nil {
					t.Fatalf("keyring.Set: %v", err)
				}
			}

			cred, err := Current(context.Background())
			if err != nil {
				t.Fatalf("Current: %v", err)
			}
			if cred.Kind != tt.wantKind {
				t.Fatalf("Kind = %v, want %v", cred.Kind, tt.wantKind)
			}
			token, err := cred.TokenSource.Token()
			if err != nil {
				t.Fatalf("Token: %v", err)
			}
			if token.AccessToken != tt.wantAccess {
				t.Fatalf("AccessToken = %q, want %q", token.AccessToken, tt.wantAccess)
			}
		})
	}
}

func TestCurrentEnvTokenBypassesKeychain(t *testing.T) {
	keyring.MockInitWithError(errors.New("keychain locked"))
	t.Cleanup(keyring.MockInit)
	t.Setenv(envKey, "env-token")

	cred, err := Current(context.Background())
	if err != nil {
		t.Fatalf("Current = %v, want the env token without reading the keychain", err)
	}
	token, err := cred.TokenSource.Token()
	if err != nil || !token.Valid() || token.AccessToken != "env-token" {
		t.Fatalf("Token() = %+v, %v; want a static, valid env token", token, err)
	}

	keyring.MockInit()
	if _, err := loadStoredToken(); err == nil {
		t.Fatal("Current wrote the env token to the keychain, it should not have")
	}
}

func TestCurrentErrors(t *testing.T) {
	t.Run("nothing stored", func(t *testing.T) {
		freshKeychain(t)
		t.Setenv(envKey, "")

		_, err := Current(context.Background())
		if !errors.Is(err, ErrNotLoggedIn) || err.Error() != "no token found; run: oku auth login" {
			t.Fatalf("err = %v, want ErrNotLoggedIn with the login hint", err)
		}
	})

	t.Run("corrupt entry", func(t *testing.T) {
		freshKeychain(t)
		t.Setenv(envKey, "")
		if err := keyring.Set(serviceName, accountName, "{not json"); err != nil {
			t.Fatalf("keyring.Set: %v", err)
		}

		_, err := Current(context.Background())
		if !errors.Is(err, ErrNotLoggedIn) || err.Error() != "stored token was corrupt and has been cleared; run: oku auth login" {
			t.Fatalf("err = %v, want ErrNotLoggedIn for a cleared corrupt entry", err)
		}
		if _, err := keyring.Get(serviceName, accountName); !errors.Is(err, keyring.ErrNotFound) {
			t.Fatalf("corrupt entry err = %v, want it deleted", err)
		}
	})

	t.Run("keyring unavailable", func(t *testing.T) {
		keyring.MockInitWithError(errors.New("keychain locked"))
		t.Cleanup(keyring.MockInit)
		t.Setenv(envKey, "")

		_, err := Current(context.Background())
		if !errors.Is(err, ErrKeyringUnavailable) || errors.Is(err, ErrNotLoggedIn) {
			t.Fatalf("err = %v, want ErrKeyringUnavailable", err)
		}
		if want := "keyring backend unavailable: keychain locked; set HARDCOVER_TOKEN as a workaround"; err.Error() != want {
			t.Fatalf("err = %q, want %q", err, want)
		}
	})
}

func TestCurrentBrowserLoginRefreshesAndPersists(t *testing.T) {
	freshKeychain(t)
	t.Setenv(envKey, "")
	rs := newRotatingServer(t, "r0")
	mustStore(t, expiredToken("old", "r0"))

	cred, err := Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	src, ok := cred.TokenSource.(*persistingTokenSource)
	if !ok {
		t.Fatalf("TokenSource = %T, want *persistingTokenSource", cred.TokenSource)
	}
	src.conf.Endpoint.TokenURL = rs.URL

	got, err := src.Token()
	if err != nil || got.AccessToken != "access-1" {
		t.Fatalf("Token() = %+v, %v; want the refreshed token", got, err)
	}
	if stored, err := loadStoredToken(); err != nil || stored.AccessToken != "access-1" {
		t.Fatalf("stored token = %+v, %v; want the refresh persisted", stored, err)
	}
}

// revokeServer counts revoke requests and answers them with status.
func revokeServer(t *testing.T, status int) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func assertKeychainEmpty(t *testing.T) {
	t.Helper()
	for _, account := range []string{accountName, legacyAccountName} {
		if _, err := keyring.Get(serviceName, account); !errors.Is(err, keyring.ErrNotFound) {
			t.Fatalf("keyring.Get(%q) err = %v, want ErrNotFound", account, err)
		}
	}
}

func TestLogoutReport(t *testing.T) {
	tests := []struct {
		name         string
		env          string
		stored       *oauth2.Token
		legacy       string
		revokeStatus int
		want         LogoutReport
		wantKey      string
		wantRevokes  int32
		wantRevokeEr bool
		wantManual   bool
	}{
		{
			name:         "browser login revoked",
			stored:       validToken("oauth", "refresh"),
			revokeStatus: http.StatusOK,
			want:         LogoutReport{Kind: KindBrowserLogin},
			wantRevokes:  1,
		},
		{
			name:         "browser login revoke fails",
			stored:       validToken("oauth", "refresh"),
			revokeStatus: http.StatusInternalServerError,
			want:         LogoutReport{Kind: KindBrowserLogin},
			wantRevokes:  1,
			wantRevokeEr: true,
		},
		{
			name:       "personal API key",
			stored:     &oauth2.Token{AccessToken: "hc_pat_abc"},
			want:       LogoutReport{Kind: KindPersonalAPIKey},
			wantManual: true,
		},
		{
			name:       "legacy JWT",
			stored:     &oauth2.Token{AccessToken: legacyJWT},
			want:       LogoutReport{Kind: KindLegacyAPIKey},
			wantKey:    legacyJWT,
			wantManual: true,
		},
		{
			name:       "legacy keychain entry",
			legacy:     legacyJWT,
			want:       LogoutReport{Kind: KindLegacyAPIKey},
			wantKey:    legacyJWT,
			wantManual: true,
		},
		{
			name: "nothing stored",
			want: LogoutReport{Kind: KindNone},
		},
		{
			name:   "env token set",
			env:    "env-token",
			stored: &oauth2.Token{AccessToken: "hc_pat_abc"},
			// the stored login is still cleared, not the env token
			want:       LogoutReport{Kind: KindPersonalAPIKey, EnvTokenSet: true},
			wantManual: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			freshKeychain(t)
			t.Setenv(envKey, tt.env)
			if tt.stored != nil {
				mustStore(t, tt.stored)
			}
			if tt.legacy != "" {
				if err := keyring.Set(serviceName, legacyAccountName, tt.legacy); err != nil {
					t.Fatalf("keyring.Set: %v", err)
				}
			}
			status := tt.revokeStatus
			if status == 0 {
				status = http.StatusOK
			}
			srv, calls := revokeServer(t, status)

			var hooked LogoutReport
			var gotKey string
			hookCalls := 0
			report, err := logoutAt(context.Background(), srv.URL, func(r LogoutReport, legacyKey string) {
				hookCalls++
				hooked, gotKey = r, legacyKey
			})
			if err != nil {
				t.Fatalf("logoutAt: %v", err)
			}
			if hookCalls != 1 {
				t.Fatalf("beforeClear called %d times, want 1", hookCalls)
			}
			if gotKey != tt.wantKey {
				t.Fatalf("legacyKey = %q, want %q", gotKey, tt.wantKey)
			}
			if hooked.Kind != report.Kind || hooked.EnvTokenSet != report.EnvTokenSet || (hooked.RevokeErr == nil) != (report.RevokeErr == nil) {
				t.Fatalf("beforeClear saw %+v, Logout returned %+v", hooked, report)
			}
			if (report.RevokeErr != nil) != tt.wantRevokeEr {
				t.Fatalf("RevokeErr = %v, want error %v", report.RevokeErr, tt.wantRevokeEr)
			}
			report.RevokeErr = nil
			if report != tt.want {
				t.Fatalf("report = %+v, want %+v", report, tt.want)
			}
			if report.ManualRevoke() != tt.wantManual {
				t.Fatalf("ManualRevoke() = %v, want %v", report.ManualRevoke(), tt.wantManual)
			}
			if got := atomic.LoadInt32(calls); got != tt.wantRevokes {
				t.Fatalf("revoke requests = %d, want %d", got, tt.wantRevokes)
			}
			assertKeychainEmpty(t)
		})
	}
}

func TestLogoutCorruptEntryIsClearedWithoutHints(t *testing.T) {
	freshKeychain(t)
	t.Setenv(envKey, "")
	if err := keyring.Set(serviceName, accountName, "{not json"); err != nil {
		t.Fatalf("keyring.Set: %v", err)
	}
	srv, calls := revokeServer(t, http.StatusOK)

	report, err := logoutAt(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("logoutAt: %v", err)
	}
	if report != (LogoutReport{}) || atomic.LoadInt32(calls) != 0 {
		t.Fatalf("report = %+v, revokes = %d; want an empty report and no revoke", report, *calls)
	}
	assertKeychainEmpty(t)
}

func TestLogoutKeyringUnavailable(t *testing.T) {
	keyring.MockInitWithError(errors.New("keychain locked"))
	t.Cleanup(keyring.MockInit)
	t.Setenv(envKey, "env-token")
	srv, calls := revokeServer(t, http.StatusOK)

	hookCalls := 0
	report, err := logoutAt(context.Background(), srv.URL, func(LogoutReport, string) { hookCalls++ })
	if hookCalls != 1 {
		t.Fatalf("beforeClear called %d times, want 1 so the env warning still shows", hookCalls)
	}
	if err == nil || !strings.Contains(err.Error(), "keychain locked") {
		t.Fatalf("err = %v, want the keyring error", err)
	}
	if want := (LogoutReport{EnvTokenSet: true}); report != want {
		t.Fatalf("report = %+v, want %+v", report, want)
	}
	if got := atomic.LoadInt32(calls); got != 0 {
		t.Fatalf("revoke requests = %d, want 0", got)
	}
}

// Aborting at the legacy key prompt must not lose the key, so the hook runs
// while it is still stored.
func TestLogoutHookRunsBeforeKeychainIsCleared(t *testing.T) {
	for _, tt := range []struct {
		name    string
		account string
		value   string
	}{
		{name: "stored token", account: accountName, value: `{"access_token":"` + legacyJWT + `"}`},
		{name: "legacy keychain entry", account: legacyAccountName, value: legacyJWT},
	} {
		t.Run(tt.name, func(t *testing.T) {
			freshKeychain(t)
			t.Setenv(envKey, "")
			if err := keyring.Set(serviceName, tt.account, tt.value); err != nil {
				t.Fatalf("keyring.Set: %v", err)
			}
			srv, _ := revokeServer(t, http.StatusOK)

			ran := false
			_, err := logoutAt(context.Background(), srv.URL, func(_ LogoutReport, legacyKey string) {
				ran = true
				if legacyKey != legacyJWT {
					t.Errorf("legacyKey = %q, want %q", legacyKey, legacyJWT)
				}
				if got, err := keyring.Get(serviceName, tt.account); err != nil || got != tt.value {
					t.Errorf("during beforeClear keyring.Get = %q, %v; want the key still stored", got, err)
				}
			})
			if err != nil {
				t.Fatalf("logoutAt: %v", err)
			}
			if !ran {
				t.Fatal("beforeClear was not called")
			}
			assertKeychainEmpty(t)
		})
	}
}

func TestKindString(t *testing.T) {
	if got := KindPersonalAPIKey.String(); got != "personal API key" {
		t.Fatalf("KindPersonalAPIKey.String() = %q", got)
	}
	if got := Kind(99).String(); got != "Kind(99)" {
		t.Fatalf("Kind(99).String() = %q", got)
	}
}
