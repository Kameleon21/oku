package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strconv"
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

var statusCodeRe = regexp.MustCompile(`status code:\s*(\d{3})`)

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

// NewClient creates a new Hardcover API client with the given auth token.
func NewClient(token string) *Client {
	httpClient := &http.Client{Timeout: requestTimeout}
	return &Client{
		gql:   graphql.NewClient(endpoint, graphql.WithHTTPClient(httpClient)),
		token: token,
	}
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

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	status := extractHTTPStatus(err)
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
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

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	status := extractHTTPStatus(err)
	return status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= http.StatusInternalServerError
}

func extractHTTPStatus(err error) int {
	if err == nil {
		return 0
	}

	msg := err.Error()
	match := statusCodeRe.FindStringSubmatch(strings.ToLower(msg))
	if len(match) != 2 {
		return 0
	}

	code, convErr := strconv.Atoi(match[1])
	if convErr != nil {
		return 0
	}
	return code
}
