package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/machinebox/graphql"
)

func TestNormalizeToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "already has Bearer prefix",
			input: "Bearer abc123",
			want:  "Bearer abc123",
		},
		{
			name:  "raw token without prefix",
			input: "abc123",
			want:  "Bearer abc123",
		},
		{
			name:  "lowercase bearer is normalised",
			input: "bearer abc123",
			want:  "Bearer abc123",
		},
		{
			name:  "uppercase BEARER is normalised",
			input: "BEARER abc123",
			want:  "Bearer abc123",
		},
		{
			name:  "mixed case Bearer is normalised",
			input: "bEaReR abc123",
			want:  "Bearer abc123",
		},
		{
			name:  "Bearer without space is treated as raw token",
			input: "BearerNoSpace",
			want:  "Bearer BearerNoSpace",
		},
		{
			name:  "surrounding whitespace is trimmed",
			input: "  abc123\n",
			want:  "Bearer abc123",
		},
		{
			name:  "whitespace around an existing prefix is trimmed",
			input: "\n Bearer  abc123 \t",
			want:  "Bearer abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeToken(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeToken(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func testClientForServer(handler http.HandlerFunc) (*Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	return newClientWithEndpoint(srv.URL, "test-token"), srv
}

func TestDoRetriesOn429(t *testing.T) {
	var calls int32
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	defer srv.Close()

	var resp struct{}
	if err := c.do(context.Background(), graphql.NewRequest(`query { me { id } }`), &resp); err != nil {
		t.Fatalf("do() = %v, want success after retry", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("server calls = %d, want 2 (429 then 200)", got)
	}
}

func TestDoExhausted429IsNetworkError(t *testing.T) {
	var calls int32
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	})
	defer srv.Close()

	var resp struct{}
	err := c.do(context.Background(), graphql.NewRequest(`query { me { id } }`), &resp)
	if err == nil {
		t.Fatal("do() = nil, want error")
	}
	if !IsNetworkError(err) {
		t.Fatalf("IsNetworkError(%v) = false, want true", err)
	}
	if got := atomic.LoadInt32(&calls); got != maxRetries {
		t.Fatalf("server calls = %d, want %d", got, maxRetries)
	}
}

func TestDoDoesNotRetryClientError(t *testing.T) {
	var calls int32
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	})
	defer srv.Close()

	var resp struct{}
	err := c.do(context.Background(), graphql.NewRequest(`query { me { id } }`), &resp)
	if err == nil {
		t.Fatal("do() = nil, want error")
	}
	if IsNetworkError(err) {
		t.Fatalf("IsNetworkError(%v) = true, want false for 400", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("server calls = %d, want 1 (no retry on 4xx)", got)
	}
}

func TestSearchBooksSendsQueryAsVariable(t *testing.T) {
	var body struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"search":{"results":{"hits":[]}}}}`))
	})
	defer srv.Close()

	tricky := "line1\nline2 \"quoted\" back\\slash"
	if _, err := c.SearchBooks(context.Background(), tricky, 10, model.SearchModeBook); err != nil {
		t.Fatalf("SearchBooks: %v", err)
	}

	if got := body.Variables["query"]; got != tricky {
		t.Fatalf("query variable = %q, want %q", got, tricky)
	}
	if strings.Contains(body.Query, "line1") {
		t.Fatal("user input was interpolated into the query document")
	}
}

func TestExtractHTTPStatusFromWrappedError(t *testing.T) {
	err := &NetworkError{Err: &StatusError{Code: http.StatusTooManyRequests}}
	if got := extractHTTPStatus(err); got != http.StatusTooManyRequests {
		t.Fatalf("extractHTTPStatus = %d, want 429", got)
	}
	if got := extractHTTPStatus(errors.New("status code: 429")); got != 0 {
		t.Fatalf("extractHTTPStatus on plain error = %d, want 0", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{name: "seconds", input: "5", want: 5 * time.Second},
		{name: "padded seconds", input: " 2 ", want: 2 * time.Second},
		{name: "zero", input: "0", want: 0},
		{name: "clamped to the cap", input: "600", want: maxRetryAfter},
		{name: "absent", input: "", want: 0},
		{name: "http-date form is ignored", input: "Wed, 21 Oct 2015 07:28:00 GMT", want: 0},
		{name: "negative", input: "-3", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRetryAfter(tt.input); got != tt.want {
				t.Fatalf("parseRetryAfter(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDoHonoursRetryAfterOver429Backoff(t *testing.T) {
	// Retry-After must exceed the 1s throttle interval, otherwise the throttle
	// alone would produce the same gap and the assertion would prove nothing.
	var mu sync.Mutex
	var seen []time.Time
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, time.Now())
		n := len(seen)
		mu.Unlock()

		if n == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	defer srv.Close()

	var resp struct{}
	if err := c.do(context.Background(), graphql.NewRequest(`query { me { id } }`), &resp); err != nil {
		t.Fatalf("do() = %v, want success after retry", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("server calls = %d, want 2", len(seen))
	}
	// The default first-retry backoff is 200ms and the throttle caps the gap
	// at 1s, so anything under ~2s means Retry-After was ignored.
	if gap := seen[1].Sub(seen[0]); gap < 1900*time.Millisecond {
		t.Fatalf("gap between attempts = %v, want >= 1.9s from Retry-After: 2", gap)
	}
}

func TestDoFailsFastWhenRetryAfterOutlastsTheDeadline(t *testing.T) {
	var calls int32
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, "rate limited: slow down")
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var resp struct{}
	start := time.Now()
	err := c.do(ctx, graphql.NewRequest(`query { me { id } }`), &resp)
	if err == nil {
		t.Fatal("do() = nil, want error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("do() returned after %v, want a fast failure rather than sleeping into the deadline", elapsed)
	}
	if !IsNetworkError(err) {
		t.Fatalf("IsNetworkError(%v) = false, want true", err)
	}
	// The point of failing fast is keeping the server's own explanation
	// instead of replacing it with a bare deadline error.
	if !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "rate limited: slow down") {
		t.Fatalf("error = %q, want it to keep the 429 status and body", err.Error())
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("server calls = %d, want 1 (no point retrying past the deadline)", got)
	}
}

func TestStatusErrorCarriesBoundedBodyAndRetryAfter(t *testing.T) {
	body := "boom: " + strings.Repeat("x", maxErrorBody)
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, body)
	})
	defer srv.Close()

	var resp struct{}
	err := c.do(context.Background(), graphql.NewRequest(`query { me { id } }`), &resp)
	if err == nil {
		t.Fatal("do() = nil, want error")
	}

	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("do() = %v, want a *StatusError in the chain", err)
	}
	if statusErr.Code != http.StatusBadRequest {
		t.Fatalf("StatusError.Code = %d, want 400", statusErr.Code)
	}
	if !strings.HasPrefix(statusErr.Body, "boom: ") {
		t.Fatalf("StatusError.Body = %q, want it to start with the response body", statusErr.Body)
	}
	if len(statusErr.Body) > maxErrorBody {
		t.Fatalf("StatusError.Body is %d bytes, want at most %d", len(statusErr.Body), maxErrorBody)
	}
	if statusErr.RetryAfter != 7*time.Second {
		t.Fatalf("StatusError.RetryAfter = %v, want 7s", statusErr.RetryAfter)
	}
	if !strings.Contains(err.Error(), "boom: ") {
		t.Fatalf("error message %q does not include the body prefix", err.Error())
	}
}

func TestDoAbortsThrottleOnCancelledContext(t *testing.T) {
	var calls int32
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	defer srv.Close()

	var resp struct{}
	// The first request arms the throttle, so the next one has to wait ~1s.
	if err := c.do(context.Background(), graphql.NewRequest(`query { me { id } }`), &resp); err != nil {
		t.Fatalf("first do() = %v, want success", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := c.do(ctx, graphql.NewRequest(`query { me { id } }`), &resp)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("do() = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("do() returned after %v, want a prompt return instead of sleeping out the interval", elapsed)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("server calls = %d, want 1 (the cancelled request must not be sent)", got)
	}
}

func TestDoSetsUserAgent(t *testing.T) {
	var got string
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		got = r.UserAgent()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	defer srv.Close()

	var resp struct{}
	if err := c.do(context.Background(), graphql.NewRequest(`query { me { id } }`), &resp); err != nil {
		t.Fatalf("do() = %v, want success", err)
	}
	if got != userAgent() {
		t.Fatalf("User-Agent = %q, want %q", got, userAgent())
	}
	if !strings.HasPrefix(got, "oku/") {
		t.Fatalf("User-Agent = %q, want it to start with %q", got, "oku/")
	}
}
