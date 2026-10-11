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
	token = strings.TrimSpace(token)
	if len(token) > 7 && strings.EqualFold(token[:7], "bearer ") {
		token = token[7:]
	}
	return strings.TrimSpace(token)
}

// loadStoredToken reads and decodes the token saved in the system keychain.
// A corrupt entry is deleted so the next login starts from a clean slate.
func loadStoredToken() (*oauth2.Token, error) {
	raw, err := keyring.Get(serviceName, accountName)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			token, migrateErr := migrateLegacyToken()
			if migrateErr == nil {
				return token, nil
			}
			if !errors.Is(migrateErr, keyring.ErrNotFound) {
				return nil, fmt.Errorf("%w: %w; set %s as a workaround", ErrKeyringUnavailable, migrateErr, envKey)
			}
			return nil, fmt.Errorf("no token found; %w", ErrNotLoggedIn)
		}
		return nil, fmt.Errorf("%w: %w; set %s as a workaround", ErrKeyringUnavailable, err, envKey)
	}

	var token oauth2.Token
	if err := json.Unmarshal([]byte(raw), &token); err != nil {
		_ = keyring.Delete(serviceName, accountName)
		return nil, fmt.Errorf("stored token was corrupt and has been cleared; %w", ErrNotLoggedIn)
	}
	return &token, nil
}

// migrateLegacyToken copies a plain API token from the pre-OAuth keychain
// entry into the new OAuth-shaped one. Returns keyring.ErrNotFound when
// there's nothing to migrate; any other error is a keyring backend problem.
func migrateLegacyToken() (*oauth2.Token, error) {
	raw, err := keyring.Get(serviceName, legacyAccountName)
	if err != nil {
		return nil, err
	}
	if raw = normalizeToken(raw); raw == "" {
		return nil, keyring.ErrNotFound
	}

	token := &oauth2.Token{AccessToken: raw, TokenType: "Bearer"}
	if err := SetToken(token); err != nil {
		return nil, err
	}
	return token, nil
}

// SetToken stores an OAuth token in the system keychain.
func SetToken(token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}
	return keyring.Set(serviceName, accountName, string(data))
}

// deleteToken removes the stored token from the system keychain. Deleting an
// already-absent token is not an error.
func deleteToken() error {
	var errs []error
	for _, account := range []string{accountName, legacyAccountName} {
		if err := keyring.Delete(serviceName, account); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// persistingTokenSource refreshes an expired token under a cross-process lock
// and writes the result to the keychain, since refresh tokens rotate on use.
type persistingTokenSource struct {
	ctx   context.Context
	conf  *oauth2.Config
	token *oauth2.Token
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	if p.token.Valid() {
		return p.token, nil
	}

	unlockRefresh, err := lockRefresh(p.ctx)
	defer unlockRefresh()
	switch {
	case errors.Is(err, errLockTimeout):
		stored, loadErr := loadStoredToken()
		if loadErr != nil {
			return nil, loadErr
		}
		if stored.Valid() {
			p.token = stored
			return stored, nil
		}
		return nil, fmt.Errorf("another oku process is refreshing the token: %w; try again", err)
	case err != nil && p.ctx.Err() != nil:
		return nil, err
	}

	// re-read under the lock: another oku process may have just refreshed
	if stored, err := loadStoredToken(); err == nil {
		p.token = stored
		if stored.Valid() {
			return stored, nil
		}
	}

	fresh, err := p.conf.TokenSource(p.ctx, p.token).Token()
	if err != nil {
		return nil, fmt.Errorf("token rejected or expired: %w; run: oku auth login", err)
	}
	if err := SetToken(fresh); err != nil {
		return nil, fmt.Errorf("store refreshed token: %w", err)
	}

	p.token = fresh
	return fresh, nil
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
