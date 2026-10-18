package app

import (
	"fmt"
	"testing"

	"github.com/Kameleon21/oku/internal/api"
)

func TestSyncStatsKeepsGoalsWhenTheJournalIsRefused(t *testing.T) {
	hc := newFakeHardcover()
	hc.goals = []api.APIGoal{{ID: 7, Goal: 24, Metric: "book"}}
	hc.errs = map[string]error{"ListReadingJournals": &api.StatusError{Code: 403, APIError: "insufficient_scope", Scope: "read:journal"}}
	a := newTestAppWith(t, hc)
	if err := a.store.SetState(userIDKey, "1"); err != nil {
		t.Fatal(err)
	}

	err := a.SyncStats(t.Context())
	if _, ok := MissingPermission(err); !ok {
		t.Fatalf("SyncStats err = %v, want the missing permission", err)
	}
	goals, err := a.store.ListGoals()
	if err != nil {
		t.Fatal(err)
	}
	if len(goals) != 1 || goals[0].ID != 7 {
		t.Fatalf("cached goals = %+v, want goal 7 despite the refused journal", goals)
	}
}

// A refused permission is explained with the remedy the client attached to
// it, and without one when the client knew none.
func TestMissingPermissionSaysHowToGetIt(t *testing.T) {
	refused := &api.StatusError{Code: 403, APIError: "insufficient_scope", Scope: "read:goals"}
	const why = "Hardcover hasn't granted oku the read:goals permission, needed for reading goals"
	if got, ok := MissingPermission(fmt.Errorf("goals: %w", refused)); !ok || got != why {
		t.Fatalf("MissingPermission = %q, %v; want %q", got, ok, why)
	}
	refused.Remedy = "Run the fix"
	if got, _ := MissingPermission(fmt.Errorf("goals: %w", refused)); got != why+". Run the fix" {
		t.Fatalf("MissingPermission = %q, want the remedy after the explanation", got)
	}
}
