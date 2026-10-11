package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// personalKeyPrefix marks Hardcover's personal API keys; older API keys are
// bare JWTs.
const personalKeyPrefix = "hc_pat_"

var (
	// ErrNotLoggedIn means no usable credential is stored. Errors wrapping it
	// already tell the user to run `oku auth login`.
	ErrNotLoggedIn = errors.New("run: oku auth login")

	// ErrKeyringUnavailable means the system keychain could not be read or
	// written. Errors wrapping it already suggest HARDCOVER_TOKEN instead.
	ErrKeyringUnavailable = errors.New("keyring backend unavailable")
)

// Kind says which credential oku authenticates with, and so where it came
// from.
type Kind int

const (
	// KindNone means no credential was found.
	KindNone Kind = iota
	// KindEnv is the HARDCOVER_TOKEN environment variable, which overrides
	// anything stored in the keychain.
	KindEnv
	// KindBrowserLogin is an OAuth token from `oku auth login`, refreshed and
	// re-stored on demand.
	KindBrowserLogin
	// KindPersonalAPIKey is a stored hc_pat_ key, whether from `oku auth
	// set-token` or carried over from before OAuth support.
	KindPersonalAPIKey
	// KindLegacyAPIKey is a pre-hc_pat_ JWT key stored with `oku auth
	// set-token` or carried over from before OAuth support.
	KindLegacyAPIKey
)

func (k Kind) String() string {
	switch k {
	case KindNone:
		return "none"
	case KindEnv:
		return "HARDCOVER_TOKEN"
	case KindBrowserLogin:
		return "browser login"
	case KindPersonalAPIKey:
		return "personal API key"
	case KindLegacyAPIKey:
		return "legacy API key"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Credential is the login oku should use right now.
type Credential struct {
	Kind Kind
	// TokenSource authorizes API requests. For a browser login it refreshes
	// an expired token under the cross-process lock and persists the result.
	TokenSource oauth2.TokenSource
}

// Current returns the credential to authenticate with: HARDCOVER_TOKEN when
// set, otherwise whatever the keychain holds. With nothing stored the error
// wraps ErrNotLoggedIn; with an unusable keychain it wraps
// ErrKeyringUnavailable.
func Current(ctx context.Context) (Credential, error) {
	if raw := envToken(); raw != "" {
		// Used as-is and the keychain is never touched, so the env var keeps
		// working on machines without a usable keychain backend (it has no
		// refresh token anyway, so there would be nothing to refresh).
		token := &oauth2.Token{AccessToken: raw, TokenType: "Bearer"}
		return Credential{Kind: KindEnv, TokenSource: oauth2.StaticTokenSource(token)}, nil
	}

	token, err := loadStoredToken()
	if err != nil {
		return Credential{}, err
	}
	// Timeout so a stalled refresh request can't hang forever.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 10 * time.Second})
	src := &persistingTokenSource{ctx: ctx, conf: GetConf(), token: token}
	return Credential{Kind: storedKind(token), TokenSource: src}, nil
}

// EnvTokenSet reports whether HARDCOVER_TOKEN overrides the stored login.
func EnvTokenSet() bool {
	return envToken() != ""
}

func envToken() string {
	return normalizeToken(os.Getenv(envKey))
}

// storedKind tells the stored credential kinds apart: only a browser login
// has a refresh token, and personal API keys carry a recognisable prefix.
func storedKind(token *oauth2.Token) Kind {
	switch {
	case token.RefreshToken != "":
		return KindBrowserLogin
	case strings.HasPrefix(token.AccessToken, personalKeyPrefix):
		return KindPersonalAPIKey
	default:
		return KindLegacyAPIKey
	}
}

// LogoutReport describes what Logout did, for the caller to tell the user.
type LogoutReport struct {
	// Kind is the stored credential being cleared, or KindNone when none
	// could be read.
	Kind Kind
	// RevokeErr is set when revoking the browser login with Hardcover failed;
	// the login is still cleared locally.
	RevokeErr error
	// EnvTokenSet reports that HARDCOVER_TOKEN is set; it still overrides the
	// stored login and logging out does not clear it.
	EnvTokenSet bool
}

// ManualRevoke reports whether the cleared credential is an API key, which
// oku cannot revoke and the user has to revoke on Hardcover themselves.
func (r LogoutReport) ManualRevoke() bool {
	return r.Kind == KindPersonalAPIKey || r.Kind == KindLegacyAPIKey
}

// Logout revokes a stored browser login with Hardcover where possible and
// clears every stored credential. Revoking is best-effort: a missing,
// unreadable or unrevocable token is still cleared locally.
//
// beforeClear, when not nil, runs after the revoke and before the keychain is
// cleared, so the caller can tell the user about the credential while it is
// still stored: if they abort there, nothing is lost. legacyKey is the stored
// key when the report's Kind is KindLegacyAPIKey, and empty otherwise; such
// keys can only be invalidated by pasting the key itself.
//
// The report is returned even when clearing the keychain fails, the only
// error returned.
func Logout(ctx context.Context, beforeClear func(report LogoutReport, legacyKey string)) (LogoutReport, error) {
	return logoutAt(ctx, revokeEndpoint, beforeClear)
}

func logoutAt(ctx context.Context, endpoint string, beforeClear func(LogoutReport, string)) (LogoutReport, error) {
	report := LogoutReport{EnvTokenSet: EnvTokenSet()}
	var legacyKey string

	if token, err := loadStoredToken(); err == nil {
		report.Kind = storedKind(token)
		switch report.Kind {
		case KindBrowserLogin:
			report.RevokeErr = logOutAt(ctx, endpoint, token)
		case KindLegacyAPIKey:
			legacyKey = token.AccessToken
		}
	}

	if beforeClear != nil {
		beforeClear(report, legacyKey)
	}
	return report, deleteToken()
}
