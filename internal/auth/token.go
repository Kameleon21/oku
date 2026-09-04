package auth

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

const (
	serviceName = "oku"
	accountName = "hardcover"
	envKey      = "HARDCOVER_TOKEN"
)

// normalizeToken trims surrounding whitespace and newlines, which routinely
// sneak in via `export HARDCOVER_TOKEN="$(cat token.txt)"` or a copy-paste
// into the keychain and would otherwise corrupt the Authorization header.
func normalizeToken(token string) string {
	return strings.TrimSpace(token)
}

// GetToken returns the API token using priority: env var > keychain.
func GetToken() (string, error) {
	if token := normalizeToken(os.Getenv(envKey)); token != "" {
		return token, nil
	}

	token, err := keyring.Get(serviceName, accountName)
	token = normalizeToken(token)
	if err == nil && token != "" {
		return token, nil
	}
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return "", fmt.Errorf("keyring backend unavailable: %w; set %s as a workaround", err, envKey)
	}

	return "", fmt.Errorf("no API token found. Set %s or run: oku auth set-token", envKey)
}

// SetToken stores the token in the system keychain.
func SetToken(token string) error {
	return keyring.Set(serviceName, accountName, token)
}

// PromptToken reads a token interactively from stdin. On a terminal the
// input is not echoed so the secret stays out of the screen and scrollback.
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
