package api

import (
	"errors"
	"net/http"
	"testing"
)

func TestStatusErrorParsing(t *testing.T) {
	tests := []struct {
		name       string
		code       int
		body       string
		is         []error
		isNot      []error
		apiError   string
		desc       string
		scope      string
		wantErrMsg string
	}{
		{
			name:       "invalid token",
			code:       401,
			body:       `{"error":"invalid_token","error_description":"Invalid or expired token"}`,
			is:         []error{ErrUnauthorized},
			isNot:      []error{ErrForbidden, ErrRateLimited},
			apiError:   "invalid_token",
			desc:       "Invalid or expired token",
			wantErrMsg: "unexpected HTTP status 401 (Unauthorized): invalid_token: Invalid or expired token",
		},
		{
			name:     "insufficient scope",
			code:     403,
			body:     `{"error":"insufficient_scope","error_description":"missing","scope":"books:write"}`,
			is:       []error{ErrForbidden, ErrInsufficientScope},
			isNot:    []error{ErrUnsupportedOp, ErrUnauthorized},
			apiError: "insufficient_scope",
			desc:     "missing",
			scope:    "books:write",
		},
		{
			name:     "unsupported operation",
			code:     403,
			body:     `{"error":"unsupported_operation"}`,
			is:       []error{ErrForbidden, ErrUnsupportedOp},
			apiError: "unsupported_operation",
		},
		{
			name:     "top level limit via errors array",
			code:     403,
			body:     `{"errors":[{"message":"max 5","extensions":{"code":"top_level_limit_exceeded"}}]}`,
			is:       []error{ErrForbidden, ErrTopLevelLimit},
			apiError: "top_level_limit_exceeded",
			desc:     "max 5",
		},
		{
			name:     "over capacity: errors array of strings",
			code:     403,
			body:     `{"errors":["request_exceeds_capacity"],"message":"8 fields exceeds burst of 5"}`,
			is:       []error{ErrForbidden, ErrOverCapacity},
			isNot:    []error{ErrTopLevelLimit},
			apiError: "request_exceeds_capacity",
			desc:     "8 fields exceeds burst of 5",
		},
		{
			name:     "rate limited uses message",
			code:     429,
			body:     `{"error":"Too Many Requests","message":"slow down"}`,
			is:       []error{ErrRateLimited},
			apiError: "Too Many Requests",
			desc:     "slow down",
		},
		{
			name:  "non-JSON body keeps raw text",
			code:  503,
			body:  "<html>\n  down\n</html>",
			is:    []error{ErrUnavailable},
			isNot: []error{ErrRateLimited},
		},
		{name: "timeout", code: 408, body: `{"error":"Request timeout"}`, is: []error{ErrTimeout}, apiError: "Request timeout"},
		{name: "not found", code: 404, is: []error{ErrNotFound}},
		{name: "bad request", code: 400, body: `{"error":"invalid_query","error_description":"x"}`, is: []error{ErrBadRequest}, apiError: "invalid_query", desc: "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &StatusError{Code: tt.code}
			e.parseErrorBody([]byte(tt.body))
			if e.APIError != tt.apiError || e.Description != tt.desc || e.Scope != tt.scope {
				t.Fatalf("parsed = (%q, %q, %q), want (%q, %q, %q)",
					e.APIError, e.Description, e.Scope, tt.apiError, tt.desc, tt.scope)
			}
			wrapped := error(&NetworkError{Err: e})
			for _, want := range tt.is {
				if !errors.Is(wrapped, want) {
					t.Errorf("errors.Is(%v) = false, want true", want)
				}
			}
			for _, not := range tt.isNot {
				if errors.Is(wrapped, not) {
					t.Errorf("errors.Is(%v) = true, want false", not)
				}
			}
			if tt.wantErrMsg != "" && e.Error() != tt.wantErrMsg {
				t.Errorf("Error() = %q, want %q", e.Error(), tt.wantErrMsg)
			}
		})
	}
}

func TestStatusErrorMessageFallsBackToBody(t *testing.T) {
	e := &StatusError{Code: http.StatusServiceUnavailable, Body: "down"}
	want := "unexpected HTTP status 503 (Service Unavailable): down"
	if e.Error() != want {
		t.Fatalf("Error() = %q, want %q", e.Error(), want)
	}
}
