package model

import (
	"reflect"
	"testing"
)

func TestGoalInputValidate(t *testing.T) {
	valid := GoalInput{Goal: 20, Metric: "book", StartDate: "2026-01-01", EndDate: "2026-12-31", PrivacySettingID: 3}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid goal rejected: %v", err)
	}
	for name, mutate := range map[string]func(*GoalInput){
		"zero goal":       func(g *GoalInput) { g.Goal = 0 },
		"unknown metric":  func(g *GoalInput) { g.Metric = "minutes" },
		"bad start":       func(g *GoalInput) { g.StartDate = "2026/01/01" },
		"reversed dates":  func(g *GoalInput) { g.EndDate = "2025-01-01" },
		"invalid privacy": func(g *GoalInput) { g.PrivacySettingID = 0 },
	} {
		g := valid
		mutate(&g)
		if g.Validate() == nil {
			t.Errorf("%s: accepted %+v", name, g)
		}
	}
}

func TestParseRatingDistribution(t *testing.T) {
	want := []RatingBucket{{5, 10}, {4.5, 2}, {4, 5}}
	for name, raw := range map[string]string{
		"array":  `[{"rating":4,"count":5},{"rating":5,"count":10},{"rating":4.5,"count":2}]`,
		"object": `{"4":5,"5":10,"4.5":2}`,
	} {
		got, err := ParseRatingDistribution(raw)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, %v; want %v", name, got, err, want)
		}
	}
	for _, raw := range []string{"", "null"} {
		if got, err := ParseRatingDistribution(raw); got != nil || err != nil {
			t.Errorf("%q: got %v, %v; want nothing", raw, got, err)
		}
	}
	for _, raw := range []string{`[{"rating":6,"count":1}]`, `{"x":1}`, `"nope"`} {
		if _, err := ParseRatingDistribution(raw); err == nil {
			t.Errorf("%q: accepted", raw)
		}
	}
}

func TestLiveRatingDistributionShape(t *testing.T) {
	rows, err := ParseRatingDistribution(`[{"count":1,"rating":3.75},{"count":1655,"rating":5.0},{"count":18,"rating":0.5}]`)
	if err != nil || len(rows) != 3 || rows[0].Rating != 5 || rows[1].Rating != 3.75 {
		t.Fatal(rows, err)
	}
	if _, err := ParseRatingDistribution(`[{"count":-1,"rating":5}]`); err == nil {
		t.Fatal("accepted negative count")
	}
}
