package app

import (
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
