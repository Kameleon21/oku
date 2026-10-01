package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

// TestMain keeps tests off the real keychain and data dir.
func TestMain(m *testing.M) {
	keyring.MockInit()
	// helper processes share their parent's data dir
	dataDir := os.Getenv("OKU_TEST_DATA_DIR")
	if dataDir == "" {
		var err error
		if dataDir, err = os.MkdirTemp("", "oku-auth-test"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	os.Setenv("XDG_DATA_HOME", dataDir)
	code := m.Run()
	if os.Getenv("OKU_TEST_DATA_DIR") == "" {
		os.RemoveAll(dataDir)
	}
	os.Exit(code)
}

// freshKeychain empties the mock keychain.
func freshKeychain(t *testing.T) {
	t.Helper()
	keyring.MockInit()
}

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
	if got.AccessToken != "env-token" {
		t.Fatalf("GetToken().AccessToken = %q, want %q", got.AccessToken, "env-token")
	}
}

func TestGetTokenEnvTokenHasNoExpiry(t *testing.T) {
	t.Setenv(envKey, "env-token")

	got, err := GetToken()
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if !got.Valid() {
		t.Fatalf("GetToken().Valid() = false, want true for a static env token")
	}
}

// rotatingServer mimics Hardcover's token endpoint: each refresh token works
// once, and replaying a spent one is rejected.
type rotatingServer struct {
	*httptest.Server
	mu      sync.Mutex
	current string // the only refresh token still accepted
	calls   int
	seq     int
}

func newRotatingServer(t *testing.T, refreshToken string) *rotatingServer {
	t.Helper()
	rs := &rotatingServer{current: refreshToken}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		rs.mu.Lock()
		defer rs.mu.Unlock()
		rs.calls++

		w.Header().Set("Content-Type", "application/json")
		if r.PostForm.Get("refresh_token") != rs.current {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		rs.seq++
		rs.current = fmt.Sprintf("refresh-%d", rs.seq)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  fmt.Sprintf("access-%d", rs.seq),
			"refresh_token": rs.current,
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *rotatingServer) callCount() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.calls
}

func (rs *rotatingServer) source(token *oauth2.Token) *persistingTokenSource {
	conf := GetConf()
	conf.Endpoint.TokenURL = rs.URL
	return &persistingTokenSource{ctx: context.Background(), conf: conf, token: token}
}

func expiredToken(access, refresh string) *oauth2.Token {
	return &oauth2.Token{AccessToken: access, RefreshToken: refresh, Expiry: time.Now().Add(-time.Hour)}
}

func validToken(access, refresh string) *oauth2.Token {
	return &oauth2.Token{AccessToken: access, RefreshToken: refresh, Expiry: time.Now().Add(time.Hour)}
}

func mustStore(t *testing.T, token *oauth2.Token) {
	t.Helper()
	if err := SetToken(token); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
}

func TestPersistingTokenSourceValidTokenSkipsKeychainAndServer(t *testing.T) {
	freshKeychain(t)
	rs := newRotatingServer(t, "r0")
	mustStore(t, validToken("stored", "r0"))

	got, err := rs.source(validToken("memory", "r0")).Token()
	if err != nil {
		t.Fatalf("Token() = %v", err)
	}
	if got.AccessToken != "memory" {
		t.Fatalf("AccessToken = %q, want the in-memory token", got.AccessToken)
	}
	if n := rs.callCount(); n != 0 {
		t.Fatalf("token endpoint called %d times, want 0", n)
	}
}

func TestPersistingTokenSourceAdoptsValidStoredToken(t *testing.T) {
	freshKeychain(t)
	rs := newRotatingServer(t, "r0")
	mustStore(t, validToken("from-other-process", "r1"))

	src := rs.source(expiredToken("stale", "r0"))
	got, err := src.Token()
	if err != nil {
		t.Fatalf("Token() = %v", err)
	}
	if got.AccessToken != "from-other-process" {
		t.Fatalf("AccessToken = %q, want the stored token", got.AccessToken)
	}
	if n := rs.callCount(); n != 0 {
		t.Fatalf("token endpoint called %d times, want 0", n)
	}
	if src.token.AccessToken != "from-other-process" {
		t.Fatalf("in-memory token = %q, want it replaced by the stored one", src.token.AccessToken)
	}
}

func TestPersistingTokenSourceRefreshesWithStoredRefreshToken(t *testing.T) {
	freshKeychain(t)
	rs := newRotatingServer(t, "r-stored")
	mustStore(t, expiredToken("old", "r-stored"))

	// in-memory refresh token is stale; the server would reject it
	got, err := rs.source(expiredToken("old", "r-stale")).Token()
	if err != nil {
		t.Fatalf("Token() = %v", err)
	}
	if got.AccessToken != "access-1" {
		t.Fatalf("AccessToken = %q, want %q", got.AccessToken, "access-1")
	}
	if n := rs.callCount(); n != 1 {
		t.Fatalf("token endpoint called %d times, want 1", n)
	}

	stored, err := loadStoredToken()
	if err != nil || stored.AccessToken != "access-1" || stored.RefreshToken != "refresh-1" {
		t.Fatalf("stored token = %+v, %v; want the rotated pair persisted", stored, err)
	}
}

// Dashboard open in one terminal, `oku sync` in another: both hold the same
// expired token, and only the first refresh may reach the server.
func TestPersistingTokenSourcesShareRefreshAcrossProcesses(t *testing.T) {
	freshKeychain(t)
	rs := newRotatingServer(t, "r0")
	mustStore(t, expiredToken("old", "r0"))

	first := rs.source(expiredToken("old", "r0"))
	second := rs.source(expiredToken("old", "r0"))

	a, err := first.Token()
	if err != nil {
		t.Fatalf("first Token() = %v", err)
	}
	b, err := second.Token()
	if err != nil {
		t.Fatalf("second Token() = %v", err)
	}
	if a.AccessToken != "access-1" || b.AccessToken != "access-1" {
		t.Fatalf("access tokens = %q, %q; want both %q", a.AccessToken, b.AccessToken, "access-1")
	}
	if n := rs.callCount(); n != 1 {
		t.Fatalf("token endpoint called %d times, want 1 (a replay would revoke the session)", n)
	}
}

func TestPersistingTokenSourceRefreshFailure(t *testing.T) {
	t.Run("rejected refresh token", func(t *testing.T) {
		freshKeychain(t)
		rs := newRotatingServer(t, "other")
		original := expiredToken("old", "r0")
		mustStore(t, original)

		_, err := rs.source(original).Token()
		if err == nil {
			t.Fatal("Token() err = nil, want an error")
		}
		for _, want := range []string{"token rejected or expired", "oku auth login"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %q, want it to contain %q", err, want)
			}
		}
		if stored, err := loadStoredToken(); err != nil || stored.AccessToken != "old" {
			t.Fatalf("stored token = %+v, %v; want it untouched", stored, err)
		}
	})

	t.Run("expired personal token has no refresh token", func(t *testing.T) {
		freshKeychain(t)
		rs := newRotatingServer(t, "r0")
		personal := &oauth2.Token{AccessToken: "personal", Expiry: time.Now().Add(-time.Hour)}
		mustStore(t, personal)

		_, err := rs.source(personal).Token()
		if err == nil || !strings.Contains(err.Error(), "oku auth login") {
			t.Fatalf("Token() err = %v, want a hint to run oku auth login", err)
		}
		if n := rs.callCount(); n != 0 {
			t.Fatalf("token endpoint called %d times, want 0", n)
		}
	})
}

func TestPersistingTokenSourceKeychainUnreadable(t *testing.T) {
	freshKeychain(t) // nothing stored, so the re-read fails
	rs := newRotatingServer(t, "r0")

	got, err := rs.source(expiredToken("old", "r0")).Token()
	if err != nil {
		t.Fatalf("Token() = %v", err)
	}
	if got.AccessToken != "access-1" {
		t.Fatalf("AccessToken = %q, want a refresh from the in-memory token", got.AccessToken)
	}
}

func TestTokenSourceEnvTokenBypassesKeychain(t *testing.T) {
	freshKeychain(t)
	t.Setenv(envKey, "env-token")

	src := TokenSource(context.Background(), &oauth2.Token{AccessToken: "env-token"})
	token, err := src.Token()
	if err != nil {
		t.Fatalf("Token() = %v", err)
	}
	if token.AccessToken != "env-token" {
		t.Fatalf("AccessToken = %q, want %q", token.AccessToken, "env-token")
	}

	if _, err := loadStoredToken(); err == nil {
		t.Fatal("TokenSource wrote the env token to the keychain, it should not have")
	}
}

func TestStoredTokenIgnoresEnvVar(t *testing.T) {
	freshKeychain(t)
	if err := SetToken(&oauth2.Token{AccessToken: "keychain-token"}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}
	t.Setenv(envKey, "env-token")

	got, err := StoredToken()
	if err != nil {
		t.Fatalf("StoredToken: %v", err)
	}
	if got.AccessToken != "keychain-token" {
		t.Fatalf("AccessToken = %q, want %q", got.AccessToken, "keychain-token")
	}
}

func TestLoadStoredTokenMigratesLegacyToken(t *testing.T) {
	freshKeychain(t)
	if err := keyring.Set(serviceName, legacyAccountName, "  legacy-token\n"); err != nil {
		t.Fatalf("keyring.Set: %v", err)
	}

	token, err := loadStoredToken()
	if err != nil {
		t.Fatalf("loadStoredToken: %v", err)
	}
	if token.AccessToken != "legacy-token" {
		t.Fatalf("AccessToken = %q, want %q", token.AccessToken, "legacy-token")
	}

	if stored, err := loadStoredToken(); err != nil || stored.AccessToken != "legacy-token" {
		t.Fatalf("after migration, loadStoredToken() = %+v, %v", stored, err)
	}

	// legacy entry stays for rollback
	if legacy, err := keyring.Get(serviceName, legacyAccountName); err != nil || legacy != "  legacy-token\n" {
		t.Fatalf("legacy entry = %q, %v; want it left untouched", legacy, err)
	}
}
