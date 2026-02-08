package api

import (
	"context"
	"sync"
	"time"

	"github.com/machinebox/graphql"
)

const endpoint = "https://api.hardcover.app/v1/graphql"

// Client wraps the machinebox/graphql client with authentication and rate limiting.
type Client struct {
	gql   *graphql.Client
	token string

	// Simple rate limiter: track last request time and sleep if needed.
	// 60 req/min = 1 per second is a safe margin.
	mu      sync.Mutex
	lastReq time.Time
}

// NewClient creates a new Hardcover API client with the given auth token.
func NewClient(token string) *Client {
	return &Client{
		gql:   graphql.NewClient(endpoint),
		token: token,
	}
}

// do executes a GraphQL request with auth header and rate limiting.
func (c *Client) do(ctx context.Context, req *graphql.Request, resp interface{}) error {
	c.throttle()

	req.Header.Set("authorization", c.token)

	return c.gql.Run(ctx, req, resp)
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
