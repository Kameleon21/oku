package auth

import (
	"context"
	"os"
	"testing"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

// TestMain keeps tests off the real keychain.
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
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

	got, err := GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if got.AccessToken != "env-token" {
		t.Fatalf("GetToken().AccessToken = %q, want %q", got.AccessToken, "env-token")
	}
}

func TestGetTokenEnvTokenHasNoExpiry(t *testing.T) {
	t.Setenv(envKey, "env-token")

	got, err := GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if !got.Valid() {
		t.Fatalf("GetToken().Valid() = false, want true for a static env token")
	}
}

// fakeTokenSource hands back tokens from a fixed list in order, repeating
// the last one once exhausted, standing in for conf.TokenSource's refresh
// behaviour without hitting a real token endpoint.
type fakeTokenSource struct {
	tokens []*oauth2.Token
	i      int
}

func (f *fakeTokenSource) Token() (*oauth2.Token, error) {
	token := f.tokens[f.i]
	if f.i < len(f.tokens)-1 {
		f.i++
	}
	return token, nil
}

func TestPersistingTokenSourcePersistsOnRefresh(t *testing.T) {
	freshKeychain(t)
	if err := SetToken(&oauth2.Token{AccessToken: "seed"}); err != nil {
		t.Fatalf("SetToken: %v", err)
	}

	src := &persistingTokenSource{src: &fakeTokenSource{tokens: []*oauth2.Token{
		{AccessToken: "first"},
		{AccessToken: "second"},
	}}}

	if _, err := src.Token(); err != nil {
		t.Fatalf("Token() = %v", err)
	}
	if stored, err := loadStoredToken(); err != nil || stored.AccessToken != "first" {
		t.Fatalf("after 1st Token(): loadStoredToken() = %+v, %v; want AccessToken %q", stored, err, "first")
	}

	if _, err := src.Token(); err != nil {
		t.Fatalf("Token() = %v", err)
	}
	if stored, err := loadStoredToken(); err != nil || stored.AccessToken != "second" {
		t.Fatalf("after 2nd Token(): loadStoredToken() = %+v, %v; want AccessToken %q", stored, err, "second")
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
