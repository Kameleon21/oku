package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kameleon21/oku/internal/tui"
)

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

// TestConfigSurvivesABadTheme: a `theme` value the loader rejects must not
// lock the user out of the commands that show and repair the config. Only the
// commands that draw with the palette refuse to start.
func TestConfigSurvivesABadTheme(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Cleanup(func() { _ = tui.ApplyThemeSetting("auto") })
	if err := os.MkdirAll(filepath.Join(home, "oku"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "oku", "config.toml"), []byte("theme = \"bogus\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		cmd := newRootCmd("test")
		cmd.SetArgs([]string{"config", "show"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("config show with an invalid theme: %v", err)
		}
	})
	if !strings.Contains(out, "not a theme") {
		t.Fatalf("config show does not report the bad value:\n%s", out)
	}

	cmd := newRootCmd("test")
	cmd.SetArgs([]string{"reading"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("a coloured command started on an invalid theme")
	}
	if !strings.Contains(err.Error(), "config") || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("error %q does not say the config names an invalid theme", err)
	}
}

// captureStdout collects what the plain fmt.Print* in a command write, which
// is where `config show` prints.
func captureStdout(t *testing.T, run func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	prev := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = prev }()

	read := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		read <- string(b)
	}()
	run()
	_ = w.Close()
	return <-read
}
