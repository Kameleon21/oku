package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
	"golang.org/x/term"
)

const (
	serviceName = "oku"
	accountName = "hardcover-oauth"
	envKey      = "HARDCOVER_TOKEN"

	// legacyAccountName held a plain API token before OAuth support was added.
	// loadStoredToken migrates it on first read so upgrading doesn't force existing users to re-authenticate.
	legacyAccountName = "hardcover"
)

// normalizeToken trims surrounding whitespace and newlines, which routinely
// sneak in via `export HARDCOVER_TOKEN="$(cat token.txt)"` or a copy-paste
// into the keychain and would otherwise corrupt the Authorization header.
func normalizeToken(token string) string {
	return strings.TrimSpace(token)
}

// GetToken returns the token to authenticate with, priority: env var >
// keychain. It does not refresh an expired token itself; pass the result
// through TokenSource to get one that refreshes (and persists the refresh)
// on demand.
func GetToken(ctx context.Context) (*oauth2.Token, error) {
	if raw := normalizeToken(os.Getenv(envKey)); raw != "" {
		return &oauth2.Token{AccessToken: raw, TokenType: "Bearer"}, nil
	}
	return loadStoredToken()
}

// loadStoredToken reads and decodes the token saved in the system keychain.
// A corrupt entry is deleted so the next login starts from a clean slate.
func loadStoredToken() (*oauth2.Token, error) {
	raw, err := keyring.Get(serviceName, accountName)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			if token, ok := migrateLegacyToken(); ok {
				return token, nil
			}
			return nil, fmt.Errorf("no token found; run: oku auth login")
		}
		return nil, fmt.Errorf("keyring backend unavailable: %w; set %s as a workaround", err, envKey)
	}

	var token oauth2.Token
	if err := json.Unmarshal([]byte(raw), &token); err != nil {
		_ = keyring.Delete(serviceName, accountName)
		return nil, fmt.Errorf("stored token was corrupt and has been cleared; run: oku auth login")
	}
	return &token, nil
}

// migrateLegacyToken moves a plain API token from the pre-OAuth keychain
// entry into the new OAuth-shaped one. ok is false when there's nothing to
// migrate (including on a keyring error, which GetToken's next call will
// report in its usual, more informative way).
func migrateLegacyToken() (token *oauth2.Token, ok bool) {
	raw, err := keyring.Get(serviceName, legacyAccountName)
	if err != nil {
		return nil, false
	}
	if raw = normalizeToken(raw); raw == "" {
		return nil, false
	}

	token = &oauth2.Token{AccessToken: raw, TokenType: "Bearer"}
	if err := SetToken(token); err != nil {
		return nil, false
	}
	_ = keyring.Delete(serviceName, legacyAccountName)
	return token, true
}

// SetToken stores an OAuth token in the system keychain.
func SetToken(token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}
	return keyring.Set(serviceName, accountName, string(data))
}

// DeleteToken removes the stored token from the system keychain. Deleting an
// already-absent token is not an error.
func DeleteToken() error {
	err := keyring.Delete(serviceName, accountName)
	if err != nil && errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// TokenSource returns an oauth2.TokenSource backed by GetConf's endpoint
// that transparently refreshes token when it has expired, and persists any
// refreshed token back to the keychain so the next invocation of oku picks
// it up without needing to refresh again.
func TokenSource(ctx context.Context, token *oauth2.Token) oauth2.TokenSource {
	return &persistingTokenSource{src: GetConf().TokenSource(ctx, token)}
}

// persistingTokenSource wraps an oauth2.TokenSource and writes the token to
// the keychain whenever it changes, i.e. right after a refresh.
type persistingTokenSource struct {
	src  oauth2.TokenSource
	last string // last access token persisted, to avoid redundant writes
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	token, err := p.src.Token()
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w; run: oku auth login", err)
	}
	if token.AccessToken != p.last {
		if err := SetToken(token); err != nil {
			return nil, fmt.Errorf("store refreshed token: %w", err)
		}
		p.last = token.AccessToken
	}
	return token, nil
}

// PromptToken reads a token interactively from stdin. On a terminal the
// input is not echoed so the secret stays out of the screen and scrollback.
// This backs the manual `set-token` fallback for environments where the
// browser-based login flow (`oku auth login`) isn't usable.
func PromptToken() (string, error) {
	fmt.Print("Enter your Hardcover API token: ")

	var token string
	if fd := int(os.Stdin.Fd()); term.IsTerminal(fd) {
		raw, err := term.ReadPassword(fd)
		fmt.Println()
		if err != nil {
			return "", fmt.Errorf("failed to read token: %w", err)
		}
		token = string(raw)
	} else {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read token: %w", err)
		}
		token = line
	}

	token = normalizeToken(token)
	if token == "" {
		return "", fmt.Errorf("token cannot be empty")
	}
	return token, nil
}
