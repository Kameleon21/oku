package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/machinebox/graphql"
	"golang.org/x/oauth2"
)

const endpoint = "https://api.hardcover.app/v1/graphql"

const (
	requestTimeout = 30 * time.Second
	attemptTimeout = 10 * time.Second
	maxRetries     = 3

	// minRequestInterval keeps us under Hardcover's 60 req/min limit.
	minRequestInterval = time.Second

	// maxErrorBody bounds how much of a non-2xx response body is kept for
	// diagnostics; maxRetryAfter caps how long a Retry-After header may hold
	// a request back.
	maxErrorBody  = 512
	maxParseBody  = 4096
	maxRetryAfter = 30 * time.Second
)

// Version identifies the build in the User-Agent header. It defaults to "dev";
// main can overwrite it with the binary's version at startup.
var Version = "dev"

// userAgent returns the User-Agent sent with every request.
func userAgent() string {
	return "oku/" + Version + " (+https://github.com/Kameleon21/oku)"
}

// Client wraps the machinebox/graphql client with authentication and rate limiting.
type Client struct {
	gql   *graphql.Client
	token string

	// Simple rate limiter: lastReq holds when the most recently reserved
	// request goes out, so throttle can space requests one second apart.
	// 60 req/min = 1 per second is a safe margin.
	mu      sync.Mutex
	lastReq time.Time
}

// NetworkError marks transient request failures (timeouts, 429, 5xx, transport errors).
// The CLI maps these to exit code 2.
type NetworkError struct {
	Err error
}

func (e *NetworkError) Error() string {
	return e.Err.Error()
}

func (e *NetworkError) Unwrap() error {
	return e.Err
}

// IsNetworkError reports whether an error should map to network/API exit code 2.
func IsNetworkError(err error) bool {
	var netErr *NetworkError
	return errors.As(err, &netErr)
}

// StatusError reports a non-2xx HTTP response from the API. The graphql
// library never exposes the status itself, so statusTransport converts
// non-2xx responses into this typed error at the transport layer.
type StatusError struct {
	Code int
	// APIError, Description and Scope are parsed from a JSON error body;
	// all are empty when the body isn't JSON.
	APIError    string // e.g. "invalid_token", "insufficient_scope"
	Description string
	Scope       string // permissions missing, for insufficient_scope
	// Body holds up to maxErrorBody bytes of the response body, with runs of
	// whitespace collapsed, so failures carry the server's explanation
	// instead of just a status number.
	Body string
	// RetryAfter is the parsed Retry-After header, or 0 when absent or unparseable.
	RetryAfter time.Duration
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("unexpected HTTP status %d (%s)", e.Code, http.StatusText(e.Code))
	switch {
	case e.APIError != "" && e.Description != "":
		msg += ": " + e.APIError + ": " + e.Description
	case e.APIError != "":
		msg += ": " + e.APIError
	case e.Body != "":
		msg += ": " + e.Body
	}
	if e.Scope != "" {
		msg += " (scope: " + e.Scope + ")"
	}
	return msg
}

// statusTransport turns non-2xx responses into *StatusError so retry and
// classification logic can match on the status code.
type statusTransport struct {
	base http.RoundTripper
}

func (t *statusTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read more than maxErrorBody so a JSON body isn't cut mid-way
		// before parsing; only the displayed Body is truncated.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxParseBody))
		resp.Body.Close()
		statusErr := &StatusError{
			Code: resp.StatusCode,
			// The body lands on stderr verbatim, and an edge proxy's HTML 503
			// is 512 bytes of newlines and indentation, so flatten it first.
			Body:       truncate(strings.Join(strings.Fields(string(body)), " "), maxErrorBody),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
		statusErr.parseErrorBody(body)
		return nil, statusErr
	}
	return resp, nil
}

// parseRetryAfter reads the delay-seconds form of the Retry-After header,
// clamped to maxRetryAfter. The HTTP-date form and invalid values yield 0.
func parseRetryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs < 0 {
		return 0
	}
	d := time.Duration(secs) * time.Second
	if d > maxRetryAfter {
		d = maxRetryAfter
	}
	return d
}

// retryAfterDelay returns the server-requested backoff for errors carrying a
// Retry-After header worth honouring (429, or 503 when the header is present).
func retryAfterDelay(err error) time.Duration {
	var statusErr *StatusError
	if !errors.As(err, &statusErr) || statusErr.RetryAfter <= 0 {
		return 0
	}
	if statusErr.Code == http.StatusTooManyRequests || statusErr.Code == http.StatusServiceUnavailable {
		return statusErr.RetryAfter
	}
	return 0
}

// NewClient creates a new Hardcover API client with the given auth token.
// The token is normalised to include a "Bearer " prefix if missing.
func NewClient(token string) *Client {
	return newClientWithEndpoint(endpoint, token)
}

func newClientWithEndpoint(url, token string) *Client {
	return &Client{
		gql:   graphql.NewClient(url, graphql.WithHTTPClient(baseHTTPClient())),
		token: normalizeToken(token),
	}
}

// NewOAuthClient creates a Hardcover API client authorized by src. The
// Authorization header is set per-request by src's own oauth2.Transport,
// which refreshes an expired token on demand.
func NewOAuthClient(ctx context.Context, src oauth2.TokenSource) *Client {
	return newOAuthClientWithEndpoint(ctx, endpoint, src)
}

func newOAuthClientWithEndpoint(ctx context.Context, url string, src oauth2.TokenSource) *Client {
	// Routes API requests through statusTransport so non-2xx responses
	// surface as *StatusError for do's retry/backoff logic. Token-refresh
	// requests use their own ctx, set in auth.TokenSource.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, baseHTTPClient())
	return &Client{
		gql: graphql.NewClient(url, graphql.WithHTTPClient(oauth2.NewClient(ctx, src))),
	}
}

// baseHTTPClient is the plain (non-OAuth) transport shared by both
// constructors: a timeout plus status-code classification.
func baseHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   attemptTimeout,
		Transport: &statusTransport{base: http.DefaultTransport},
	}
}

// normalizeToken ensures the token carries the "Bearer " prefix required by the API.
// Surrounding whitespace is trimmed and any existing bearer prefix
// (case-insensitive) stripped before re-adding the canonical form.
func normalizeToken(token string) string {
	const prefix = "Bearer "
	token = strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return prefix + strings.TrimSpace(token[len("bearer "):])
	}
	return prefix + token
}

// do executes a GraphQL request with auth header and rate limiting.
func (c *Client) do(ctx context.Context, req *graphql.Request, resp interface{}) error {
	ctx, cancel := withRequestTimeout(ctx)
	defer cancel()

	if c.token != "" {
		req.Header.Set("authorization", c.token)
	}
	req.Header.Set("User-Agent", userAgent())

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := c.throttle(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			return &NetworkError{Err: err}
		}

		err := c.gql.Run(ctx, req, resp)
		if err == nil {
			return nil
		}
		if refreshRejected(err) {
			return fmt.Errorf("%w: %w", ErrUnauthorized, err)
		}

		if !isRetryable(err) {
			if isNetworkish(err) {
				return &NetworkError{Err: err}
			}
			return err
		}

		lastErr = err
		if attempt == maxRetries {
			break
		}

		backoff := time.Duration(attempt*attempt) * 200 * time.Millisecond
		if d := retryAfterDelay(err); d > 0 {
			backoff = d
		}
		// Sleeping out the deadline would report a bare timeout and throw away
		// lastErr — the status and body the server actually sent. A Retry-After
		// longer than the time left is a definite failure, so say so now.
		if backoff >= remaining(ctx) {
			return &NetworkError{Err: lastErr}
		}
		select {
		case <-ctx.Done():
			// A user-initiated cancel stays a cancellation; a deadline that
			// expires mid-backoff still reports what the server last said.
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			return &NetworkError{Err: lastErr}
		case <-time.After(backoff):
		}
	}

	// The loop only exits via break, which always assigns lastErr first.
	return &NetworkError{Err: lastErr}
}

// remaining reports how long ctx has left. do always runs under a deadline
// because withRequestTimeout adds one, so the fallback is only a safety net.
func remaining(ctx context.Context) time.Duration {
	deadline, ok := ctx.Deadline()
	if !ok {
		return requestTimeout
	}
	return time.Until(deadline)
}

// throttle spaces requests at least minRequestInterval apart to respect the
// 60 req/min rate limit. The slot is reserved under the lock and the wait
// happens outside it, so concurrent callers queue up rather than blocking each
// other, and a cancelled context aborts the wait.
func (c *Client) throttle(ctx context.Context) error {
	c.mu.Lock()
	var wait time.Duration
	if !c.lastReq.IsZero() {
		if elapsed := time.Since(c.lastReq); elapsed < minRequestInterval {
			wait = minRequestInterval - elapsed
		}
	}
	// Record when the request will actually go out, so a backoff that already
	// covered the interval does not trigger a second full wait.
	c.lastReq = time.Now().Add(wait)
	c.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

func withRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, requestTimeout)
}

// refreshRejected reports whether the token endpoint refused to refresh an
// expired login, e.g. a revoked or replayed refresh token. That is a dead
// login, not a network failure: retrying only replays the refresh token.
func refreshRejected(err error) bool {
	var retrieveErr *oauth2.RetrieveError
	return errors.As(err, &retrieveErr) && retrieveErr.Response != nil &&
		retrieveErr.Response.StatusCode >= 400 && retrieveErr.Response.StatusCode < 500
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// User-initiated cancellation should not be retried.
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check the HTTP status before the generic net.Error test: a *url.Error
	// wrapping a StatusError satisfies net.Error, but a 4xx is not retryable.
	if status := extractHTTPStatus(err); status != 0 {
		return status == http.StatusTooManyRequests ||
			status == http.StatusRequestTimeout ||
			status >= http.StatusInternalServerError
	}

	var netErr net.Error
	return errors.As(err, &netErr)
}

func isNetworkish(err error) bool {
	if err == nil {
		return false
	}

	// Cancellation is intentional and should not be classified as network failure.
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	if status := extractHTTPStatus(err); status != 0 {
		return status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= http.StatusInternalServerError
	}

	var netErr net.Error
	return errors.As(err, &netErr)
}

func extractHTTPStatus(err error) int {
	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		return statusErr.Code
	}
	return 0
}
