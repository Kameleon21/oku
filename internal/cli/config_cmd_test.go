package cli

import "testing"

func TestResolveEditor(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}

	tests := []struct {
		name      string
		cfgEditor string
		env       map[string]string
		want      string
	}{
		{name: "config wins", cfgEditor: "nvim", env: map[string]string{"VISUAL": "code", "EDITOR": "nano"}, want: "nvim"},
		{name: "visual before editor", env: map[string]string{"VISUAL": "code", "EDITOR": "nano"}, want: "code"},
		{name: "editor when no visual", env: map[string]string{"EDITOR": "nano"}, want: "nano"},
		{name: "vi fallback", want: "vi"},
		{name: "blank values skipped", cfgEditor: "  ", env: map[string]string{"VISUAL": " ", "EDITOR": "  nano "}, want: "nano"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveEditor(tt.cfgEditor, env(tt.env)); got != tt.want {
				t.Fatalf("resolveEditor(%q, %v) = %q, want %q", tt.cfgEditor, tt.env, got, tt.want)
			}
		})
	}
}
