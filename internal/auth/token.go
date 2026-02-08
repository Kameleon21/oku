package auth

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	serviceName = "oku"
	accountName = "hardcover"
	envKey      = "HARDCOVER_TOKEN"
)

// GetToken returns the API token using priority: env var > keychain.
func GetToken() (string, error) {
	if token := os.Getenv(envKey); token != "" {
		return token, nil
	}

	token, err := keyring.Get(serviceName, accountName)
	if err == nil && token != "" {
		return token, nil
	}

	return "", fmt.Errorf("no API token found. Set %s or run: oku auth set-token", envKey)
}

// SetToken stores the token in the system keychain.
func SetToken(token string) error {
	return keyring.Set(serviceName, accountName, token)
}

// PromptToken reads a token interactively from stdin.
func PromptToken() (string, error) {
	fmt.Print("Enter your Hardcover API token: ")
	reader := bufio.NewReader(os.Stdin)
	token, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("token cannot be empty")
	}
	return token, nil
}
