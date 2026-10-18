package api

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/machinebox/graphql"
)

// scope is a Hardcover permission oku asks for and the feature that needs it.
type scope struct {
	name, feature string
}

// scopes is the minimum oku needs, per https://api.hardcover.app/capabilities.json
var scopes = []scope{
	{"read:me:content", "your account details"},                  // me: current user id/username
	{"read:library", "your library"},                             // user_books: library, reading progress
	{"read:journal", "notes, quotes and the activity heatmap"},   // reading_journals
	{"read:goals", "reading goals"},                              // goals
	{"read:lists", "the reading queue"},                          // lists, list_books
	{"read:catalog", "search, trending and book details"},        // search, books, editions
	{"write:library", "updating your library, notes and quotes"}, // insert/update_user_book, insert_reading_journal
	{"write:reviews", "ratings and reviews"},                     // review writing
	{"write:goals", "editing reading goals"},                     // insert/update_goal
	{"write:lists", "reordering the reading queue"},              // insert_list, update_list_books
}

// ScopeNames returns every permission oku requests, in a stable order.
func ScopeNames() []string {
	names := make([]string, len(scopes))
	for i, s := range scopes {
		names[i] = s.name
	}
	return names
}

// ScopeFeature describes what a permission is needed for. A sub-scope such as
// read:journal:private describes its parent; an unknown scope gives "".
func ScopeFeature(name string) string {
	for _, s := range scopes {
		if within(name, s.name) {
			return s.feature
		}
	}
	return ""
}

// within reports whether scope is parent or one of its sub-scopes
// (read:journal:private within read:journal). Hardcover's parents imply
// their sub-scopes, never the reverse.
func within(scope, parent string) bool {
	return scope == parent || strings.HasPrefix(scope, parent+":")
}

// splitScopes reads a scope list separated by spaces or commas.
func splitScopes(s string) []string {
	return strings.Fields(strings.ReplaceAll(s, ",", " "))
}

// UngrantedScopes compares the scope list a token response reported with the
// permissions oku asked for and returns those missing, in request order. An
// empty list means the server did not report one, which OAuth defines as
// "everything requested", so nothing is missing.
func UngrantedScopes(granted string) []string {
	got := splitScopes(granted)
	if len(got) == 0 {
		return nil
	}
	var missing []string
	for _, s := range scopes {
		found := false
		for _, g := range got {
			if g == "all" || within(s.name, g) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, s.name)
		}
	}
	return missing
}

// MissingScopes returns the permissions Hardcover named when it refused a
// request for lack of them, or nil when err is anything else.
func MissingScopes(err error) []string {
	var se *StatusError
	if !errors.As(err, &se) || !se.Is(ErrInsufficientScope) {
		return nil
	}
	if s := splitScopes(se.Scope); len(s) > 0 {
		return s
	}
	// "Missing scopes: read:journal": the scope names are the words with a
	// colon inside, not at the end.
	var names []string
	for _, w := range splitScopes(se.Description) {
		if i := strings.Index(w, ":"); i > 0 && i < len(w)-1 {
			names = append(names, w)
		}
	}
	return names
}

// insufficientScopeMessage says which permission is missing and what it is
// for, in place of the raw 403. How to get it depends on the credential, so
// that is left to the caller.
func insufficientScopeMessage(missing []string) string {
	if len(missing) == 0 {
		return "Hardcover hasn't granted oku a permission this needs"
	}
	var features []string
	for _, m := range missing {
		if f := ScopeFeature(m); f != "" && !slices.Contains(features, f) {
			features = append(features, f)
		}
	}
	msg := "Hardcover hasn't granted oku the " + strings.Join(missing, ", ") + " permission"
	if len(missing) > 1 {
		msg += "s"
	}
	if len(features) > 0 {
		msg += ", needed for " + strings.Join(features, " and for ")
	}
	return msg
}

// permissionCheck returns a top-level field that reads no rows from field,
// for a query that reads field nested under me. Hardcover checks permissions
// on top-level fields only, so without it a token lacking the permission gets
// an empty list (a shelf or goals that seem to have vanished) instead of a
// refusal. The response ignores it.
func permissionCheck(field string) string {
	return "permission_check: " + field + "(limit: 0) { id }"
}

// readProbes are the cheapest reads behind each read permission oku needs
// beyond read:me:content, which any signed-in check already exercises.
var readProbes = []struct{ field, scope string }{
	{"user_books", "read:library"},
	{"reading_journals", "read:journal"},
	{"goals", "read:goals"},
	{"lists", "read:lists"},
	{"books", "read:catalog"},
}

// CheckReadScopes asks Hardcover which of oku's read permissions the token
// lacks. Write permissions can't be checked without writing, so they are not.
// It costs one request when everything is granted; Hardcover refuses a
// request over its first missing permission, so each one found costs one
// more. Errors other than a missing permission are returned as they are.
func (c *Client) CheckReadScopes(ctx context.Context) ([]string, error) {
	pending := readProbes
	var missing []string
	for len(pending) > 0 {
		fields := make([]string, len(pending))
		for i, p := range pending {
			fields[i] = p.field + "(limit: 1) { id }"
		}
		err := c.do(ctx, graphql.NewRequest("query { "+strings.Join(fields, " ")+" }"), &struct{}{})
		if err == nil {
			return missing, nil
		}
		refused := MissingScopes(err)
		if refused == nil {
			return missing, fmt.Errorf("CheckReadScopes: %w", err)
		}
		missing = append(missing, refused...)

		kept := pending[:0:0]
		for _, p := range pending {
			if !refusedAny(refused, p.scope) {
				kept = append(kept, p)
			}
		}
		if len(kept) == len(pending) {
			// A permission none of the probes stands for: report it and stop
			// rather than ask again for the same refusal.
			return missing, nil
		}
		pending = kept
	}
	return missing, nil
}

func refusedAny(refused []string, scope string) bool {
	for _, r := range refused {
		if within(r, scope) {
			return true
		}
	}
	return false
}
