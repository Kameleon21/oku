package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/Kameleon21/oku/internal/api"
)

func TestNewReaderCommandsRegistered(t *testing.T) {
	root := newRootCmd("test")
	for _, name := range []string{"note", "quote", "journal", "book", "goals", "trending", "export", "import", "paused"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd == root {
			t.Errorf("missing %s", name)
		}
	}
}
func TestGoalUpdatePreservesUnspecifiedFields(t *testing.T) {
	root := newGoalsCmd()
	set, _, _ := root.Find([]string{"set"})
	set.Flags().Set("target", "40")
	old := api.GoalInput{Goal: 20, Description: "Private challenge", Metric: "book", StartDate: "2025-06-01", EndDate: "2027-05-31", PrivacySettingID: 3, Conditions: map[string]any{"readingFormatId": 1}}
	got := mergeGoalSettings(set, old, api.GoalInput{Goal: 40})
	want := old
	want.Goal = 40
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discarded settings: %#v", got)
	}
}
func TestReaderHelpExplainsImportPreview(t *testing.T) {
	cmd := newImportCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.Help()
	if !strings.Contains(out.String(), "preview") || !strings.Contains(out.String(), "--apply") {
		t.Fatal(out.String())
	}
}
