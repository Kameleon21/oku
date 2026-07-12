package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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
