package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/machinebox/graphql"
)

const endpoint = "https://api.hardcover.app/v1/graphql"

const (
	requestTimeout = 30 * time.Second
	maxRetries     = 3
)

// Client wraps the machinebox/graphql client with authentication and rate limiting.
type Client struct {
	gql   *graphql.Client
	token string

	// Simple rate limiter: track last request time and sleep if needed.
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
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("unexpected HTTP status %d (%s)", e.Code, http.StatusText(e.Code))
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
		resp.Body.Close()
		return nil, &StatusError{Code: resp.StatusCode}
	}
	return resp, nil
}

// NewClient creates a new Hardcover API client with the given auth token.
// The token is normalised to include a "Bearer " prefix if missing.
func NewClient(token string) *Client {
	return newClientWithEndpoint(endpoint, token)
}

func newClientWithEndpoint(url, token string) *Client {
	httpClient := &http.Client{
		Timeout:   requestTimeout,
		Transport: &statusTransport{base: http.DefaultTransport},
	}
	return &Client{
		gql:   graphql.NewClient(url, graphql.WithHTTPClient(httpClient)),
		token: normalizeToken(token),
	}
}

// normalizeToken ensures the token carries the "Bearer " prefix required by the API.
// It strips any existing bearer prefix (case-insensitive) before re-adding the canonical form.
func normalizeToken(token string) string {
	const prefix = "Bearer "
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		return prefix + token[len("bearer "):]
	}
	return prefix + token
}

// do executes a GraphQL request with auth header and rate limiting.
func (c *Client) do(ctx context.Context, req *graphql.Request, resp interface{}) error {
	ctx, cancel := withRequestTimeout(ctx)
	defer cancel()

	req.Header.Set("authorization", c.token)

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		c.throttle()

		err := c.gql.Run(ctx, req, resp)
		if err == nil {
			return nil
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
		select {
		case <-ctx.Done():
			return &NetworkError{Err: ctx.Err()}
		case <-time.After(backoff):
		}
	}

	if lastErr == nil {
		lastErr = context.DeadlineExceeded
	}
	return &NetworkError{Err: lastErr}
}

// throttle ensures at least 1 second between requests to respect the 60 req/min rate limit.
func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.lastReq.IsZero() {
		elapsed := time.Since(c.lastReq)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
	}
	c.lastReq = time.Now()
}

func withRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, requestTimeout)
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
		return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
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
