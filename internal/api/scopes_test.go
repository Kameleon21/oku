package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestUngrantedScopes(t *testing.T) {
	tests := []struct {
		name    string
		granted string
		want    []string
	}{
		{"not reported", "", nil},
		{"everything", strings.Join(ScopeNames(), " "), nil},
		{"all", "all", nil},
		// The consent screen from issue #59: three read scopes dropped.
		{"issue 59", "read:me:content read:library read:catalog write:library write:reviews write:goals write:lists",
			[]string{"read:journal", "read:goals", "read:lists"}},
		// A parent implies its sub-scopes, never the reverse.
		{"parent implies sub-scope", "read:me read:library read:journal read:goals read:lists read:catalog write:library write:reviews write:goals write:lists",
			nil},
		{"sub-scopes alone don't", "read:me:content read:library:private read:journal:private read:journal:public read:goals read:lists read:catalog write:library write:reviews write:goals write:lists",
			[]string{"read:library", "read:journal"}},
		{"comma separated", "read:me:content,read:library", []string{"read:journal", "read:goals", "read:lists", "read:catalog", "write:library", "write:reviews", "write:goals", "write:lists"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UngrantedScopes(tt.granted); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("UngrantedScopes(%q) = %v, want %v", tt.granted, got, tt.want)
			}
		})
	}
}

func TestScopeFeatureCoversEveryScope(t *testing.T) {
	for _, name := range ScopeNames() {
		if ScopeFeature(name) == "" {
			t.Errorf("ScopeFeature(%q) is empty", name)
		}
	}
	if got := ScopeFeature("read:journal:private"); got != ScopeFeature("read:journal") {
		t.Errorf("a sub-scope should describe its parent's feature, got %q", got)
	}
}

func scopeError(scope string) *StatusError {
	return &StatusError{Code: http.StatusForbidden, APIError: "insufficient_scope", Description: "Missing scopes: " + scope, Scope: scope}
}

func TestMissingScopes(t *testing.T) {
	wrapped := fmt.Errorf("BookJournal: %w", scopeError("read:journal"))
	if got := MissingScopes(wrapped); !reflect.DeepEqual(got, []string{"read:journal"}) {
		t.Fatalf("MissingScopes(wrapped) = %v", got)
	}
	// No scope field: fall back to the description.
	bare := &StatusError{Code: 403, APIError: "insufficient_scope", Description: "Missing scopes: read:goals, read:lists"}
	if got := MissingScopes(bare); !reflect.DeepEqual(got, []string{"read:goals", "read:lists"}) {
		t.Fatalf("MissingScopes(description only) = %v", got)
	}
	joined := errors.Join(fmt.Errorf("journals: %w", scopeError("read:journal")), fmt.Errorf("goals: %w", scopeError("read:goals")))
	if got := MissingScopes(joined); !reflect.DeepEqual(got, []string{"read:journal", "read:goals"}) {
		t.Fatalf("MissingScopes(joined) = %v, want both", got)
	}
	withURL := &StatusError{Code: 403, APIError: "insufficient_scope", Description: "Missing scopes: read:lists; see https://docs.hardcover.app/api"}
	if got := MissingScopes(withURL); !reflect.DeepEqual(got, []string{"read:lists"}) {
		t.Fatalf("MissingScopes(description with URL) = %v", got)
	}
	prefixed := &StatusError{Code: 403, APIError: "insufficient_scope", Description: "insufficient_scope: Missing scopes: read:journal"}
	if got := MissingScopes(prefixed); !reflect.DeepEqual(got, []string{"read:journal"}) {
		t.Fatalf("MissingScopes(prefixed description) = %v", got)
	}
	if got := MissingScopes(&StatusError{Code: 403, APIError: "unsupported_operation"}); got != nil {
		t.Fatalf("MissingScopes(other 403) = %v, want nil", got)
	}
	if got := MissingScopes(errors.New("boom")); got != nil {
		t.Fatalf("MissingScopes(plain error) = %v, want nil", got)
	}
}

func TestInsufficientScopeMessage(t *testing.T) {
	got := scopeError("read:journal").Error()
	if want := "Hardcover hasn't granted oku the read:journal permission, needed for notes, quotes and the activity heatmap"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	if got, want := scopeError("read:goals read:lists").Error(), "Hardcover hasn't granted oku the read:goals, read:lists permissions, needed for reading goals and for the reading queue"; got != want {
		t.Errorf("plural message = %q, want %q", got, want)
	}
	if strings.Contains(got, "403") {
		t.Errorf("message %q still shows the raw status", got)
	}
	if !errors.Is(scopeError("read:journal"), ErrInsufficientScope) {
		t.Error("message change must keep errors.Is(ErrInsufficientScope)")
	}
}

// scopeServer answers the read-scope probe like Hardcover: a 403 naming the
// first refused field's scope, until no refused field is left.
func scopeServer(refused map[string]string, requests *[]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct{ Query string }
		_ = json.Unmarshal(body, &req)
		*requests = append(*requests, req.Query)
		for _, field := range readProbes {
			// Top-level reads only: the probe's and permissionCheck's
			// "(limit:" form, as Hardcover checks no nested field.
			if scope, ok := refused[field.field]; ok && strings.Contains(req.Query, field.field+"(limit:") {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, `{"error":"insufficient_scope","error_description":"Missing scopes: %s","scope":%q}`, scope, scope)
				return
			}
		}
		fmt.Fprint(w, `{"data":{}}`)
	}
}

func TestCheckReadScopes(t *testing.T) {
	tests := []struct {
		name         string
		refused      map[string]string
		want         []string
		wantRequests int
	}{
		{"all granted", nil, nil, 1},
		{"issue 59", map[string]string{"reading_journals": "read:journal", "goals": "read:goals", "lists": "read:lists"},
			[]string{"read:journal", "read:goals", "read:lists"}, 4},
		{"sub-scope reported", map[string]string{"reading_journals": "read:journal:private"}, []string{"read:journal:private"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []string
			c, srv := testClientForServer(scopeServer(tt.refused, &requests))
			defer srv.Close()

			got, err := c.CheckReadScopes(t.Context())
			if err != nil {
				t.Fatalf("CheckReadScopes: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("CheckReadScopes = %v, want %v", got, tt.want)
			}
			if len(requests) != tt.wantRequests {
				t.Fatalf("made %d requests, want %d", len(requests), tt.wantRequests)
			}
		})
	}
}

func TestCheckReadScopesStopsOnUnknownScope(t *testing.T) {
	var requests int
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"insufficient_scope","scope":"read:something_new"}`)
	})
	defer srv.Close()

	got, err := c.CheckReadScopes(t.Context())
	if err != nil {
		t.Fatalf("CheckReadScopes: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"read:something_new"}) || requests != 1 {
		t.Fatalf("got %v after %d requests, want [read:something_new] after 1", got, requests)
	}
}

func TestCheckReadScopesReturnsOtherErrors(t *testing.T) {
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"invalid_token"}`)
	})
	defer srv.Close()

	if _, err := c.CheckReadScopes(t.Context()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

// Reads nested under me must still be refused without their permission:
// Hardcover only checks top-level fields, and an empty shelf or goal list
// would look like data loss.
func TestMeNestedReadsAreRefusedWithoutTheirPermission(t *testing.T) {
	var requests []string
	c, srv := testClientForServer(scopeServer(map[string]string{"goals": "read:goals", "user_books": "read:library"}, &requests))
	defer srv.Close()

	if _, err := c.ListGoals(t.Context()); !reflect.DeepEqual(MissingScopes(err), []string{"read:goals"}) {
		t.Fatalf("ListGoals err = %v, want read:goals refused", err)
	}
	if _, err := c.ListUserBooks(t.Context(), 1); !reflect.DeepEqual(MissingScopes(err), []string{"read:library"}) {
		t.Fatalf("ListUserBooks err = %v, want read:library refused", err)
	}
}
