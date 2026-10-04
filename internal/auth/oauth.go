package auth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
)

const (
	// Public client (PKCE, no secret), so the ID is safe to commit.
	clientID = "41696682-fd7a-45ad-8145-eb002ac6179d"
)

// scopes is the minimum the app needs, per https://api.hardcover.app/capabilities.json
var scopes = []string{
	"read:me:content", // me: current user id/username
	"read:library",    // user_books: library, reading progress
	"read:journal",    // reading_journals: notes/quotes, activity stats
	"read:goals",      // goals: reading goals
	"read:lists",      // lists, list_books: reading queue
	"read:catalog",    // search, books, editions: search, trending, ISBN lookup
	"write:library",   // insert/update_user_book, insert_reading_journal: library writes, ratings/reviews, journal entries
	"write:reviews",   // review writing
	"write:goals",     // insert/update_goal: reading goals
	"write:lists",     // insert_list, update_list_books: reading queue
}

// const OAUTH_AUTHORIZATION_SERVER = "https://api.hardcover.app/.well-known/oauth-authorization-server"
const (
	authorizeEndpoint = "https://hardcover.app/oauth2/authorize"
	tokenEndpoint     = "https://api.hardcover.app/oauth2/token"
	revokeEndpoint    = "https://api.hardcover.app/oauth2/revoke"
	issuer            = "https://api.hardcover.app"
)

const newTokenPageURL = "https://hardcover.app/account/api/keys/new"

// NewTokenURL returns a link to Hardcover's "New API Key" form with oku's required scopes pre-checked,
// per https://docs.hardcover.app/api/pat-link-builder/
func NewTokenURL() string {
	u, err := url.Parse(newTokenPageURL)
	if err != nil {
		panic(err)
	}

	q := u.Query()
	q.Set("scope", strings.Join(scopes, " "))
	u.RawQuery = q.Encode()
	return u.String()
}

func GetConf() *oauth2.Config {
	return &oauth2.Config{
		ClientID: clientID,
		Scopes:   scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:   authorizeEndpoint,
			TokenURL:  tokenEndpoint,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
}

func Login(ctx context.Context, out io.Writer) (*oauth2.Token, error) {
	// bind to any free loopback port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("open listener: %w", err)
	}
	defer listener.Close()

	conf := GetConf()
	conf.RedirectURL = fmt.Sprintf("http://%s/callback", listener.Addr())

	state := oauth2.GenerateVerifier()
	verifier := oauth2.GenerateVerifier()

	authURL := conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	fmt.Fprintf(out, "Open this URL and approve access:\n%s\n", authURL)
	_ = browser.OpenURL(authURL) // URL was printed, ignore error

	code, err := awaitCode(ctx, listener, state)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 10 * time.Second})
	return conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
}

type callbackResult struct {
	code string
	err  error
}

// awaitCode serves the redirect endpoint until a callback for this login attempt arrives, the server fails, or ctx is cancelled.
func awaitCode(ctx context.Context, listener net.Listener, state string) (string, error) {
	results := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code, ok, err := checkCallback(r.URL.Query(), state)
		switch {
		case !ok:
			// not ours (stale tab, another session): ignore and keep waiting
			http.Error(w, "This sign-in link is stale. Check your terminal.", http.StatusBadRequest)
			return
		case err != nil:
			http.Error(w, "Sign-in failed. Check your terminal for details.", http.StatusBadRequest)
		default:
			fmt.Fprint(w, "Signed in, you can close this tab.")
		}
		select {
		case results <- callbackResult{code, err}:
		default: // already have a result; drop duplicates
		}
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(listener) }()
	defer func() {
		// graceful shutdown lets the in-flight response reach the browser
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	select {
	case res := <-results:
		return res.code, res.err
	case err := <-serveErr:
		return "", fmt.Errorf("callback server: %w", err)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// checkCallback validates a redirect query. ok is false when the request doesn't belong to this login attempt and should be ignored.
func checkCallback(q url.Values, state string) (code string, ok bool, err error) {
	if q.Get("state") != state {
		return "", false, nil
	}
	if q.Get("iss") != issuer {
		return "", true, errors.New("authorization response came from the wrong issuer")
	}
	if e := q.Get("error"); e != "" {
		errorMessage := e
		if ed := q.Get("error_description"); ed != "" {
			errorMessage = fmt.Sprintf("%s: %s", errorMessage, ed)
		}
		return "", true, fmt.Errorf("authorization failed: %s", errorMessage)
	}
	if code = q.Get("code"); code == "" {
		return "", true, errors.New("authorization response missing code")
	}
	return code, true, nil
}

func LogOut(ctx context.Context, token *oauth2.Token) error {
	return logOutAt(ctx, revokeEndpoint, token)
}

func logOutAt(ctx context.Context, endpoint string, token *oauth2.Token) error {
	if token == nil {
		return nil
	}

	if token.RefreshToken == "" {
		return nil
	}

	form := url.Values{
		"token":           {token.RefreshToken},
		"token_type_hint": {"refresh_token"},
		"client_id":       {clientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
		return fmt.Errorf("revoke token: %s: %s", resp.Status, bytes.TrimSpace(body))
	}
	return nil
}
