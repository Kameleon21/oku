package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kameleon21/oku/internal/config"
	"github.com/Kameleon21/oku/internal/tui"
)

// TestListAndPreviewNameEveryTheme: both listings have to show every value
// the config key takes, or a reader cannot pick the one they want.
func TestListAndPreviewNameEveryTheme(t *testing.T) {
	t.Cleanup(func() { _ = tui.ApplyThemeSetting("auto") })

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"list", listThemes},
		{"preview", previewThemes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureOutput(t, "TERM=xterm-256color")
			if err := tc.run(); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			got := buf.String()
			for _, name := range tui.ThemeSettings() {
				if name == "auto" && tc.name == "preview" {
					// "auto" is not a palette: it picks one of the two the
					// preview already draws.
					continue
				}
				if !strings.Contains(got, name) {
					t.Fatalf("%s output does not mention %q:\n%s", tc.name, name, got)
				}
			}
			// Off a terminal the swatches are plain blocks, not escapes.
			if strings.ContainsRune(got, 0x1b) {
				t.Fatalf("escape sequences off a terminal:\n%q", got)
			}
		})
	}
}

// TestSetThemeWritesAValidName: the name the user types is validated by the
// same rules the dashboard applies, so nothing the command writes can stop
// the next run from starting.
func TestSetThemeWritesAValidName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Cleanup(func() { _ = tui.ApplyThemeSetting("auto") })
	captureOutput(t, "TERM=xterm-256color")

	if err := setTheme("Tokyo_Night"); err != nil {
		t.Fatalf("setTheme: %v", err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// The canonical spelling is what lands in the file, whatever was typed.
	if cfg.Theme != "tokyo-night" {
		t.Fatalf("cfg.Theme = %q, want tokyo-night", cfg.Theme)
	}
	if err := tui.ApplyThemeSetting(cfg.Theme); err != nil {
		t.Fatalf("the written value does not load back: %v", err)
	}

	// A background setting is written under its own name rather than a
	// palette's.
	if err := setTheme("AUTO"); err != nil {
		t.Fatalf("setTheme(AUTO): %v", err)
	}
	if cfg, err = config.Load(); err != nil || cfg.Theme != "auto" {
		t.Fatalf("cfg.Theme = %q (err %v), want auto", cfg.Theme, err)
	}
}

// TestSetThemeRejectsAnUnknownName: nothing is written when the name is not
// one of ours, so a typo cannot break a working config.
func TestSetThemeRejectsAnUnknownName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Cleanup(func() { _ = tui.ApplyThemeSetting("auto") })
	captureOutput(t, "TERM=xterm-256color")

	if err := setTheme("gruvbox"); err == nil {
		t.Fatal("setTheme(gruvbox) = nil, want an error naming the valid values")
	} else if !strings.Contains(err.Error(), "gruvbox-dark") {
		t.Fatalf("error %q does not list the valid values", err)
	}
	path, err := config.FilePath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a rejected name still wrote %s", filepath.Base(path))
	}
}

// TestDescribeTheme is what `oku config show` prints for the key.
func TestDescribeTheme(t *testing.T) {
	for setting, want := range map[string]string{
		"":                 "auto",
		"  AUTO ":          "auto",
		"dark":             "dark",
		"Nord":             "nord",
		"tokyo_night":      "tokyo-night",
		"catppuccin-mocha": "catppuccin-mocha",
	} {
		if got := describeTheme(setting); !strings.HasPrefix(got, want) {
			t.Fatalf("describeTheme(%q) = %q, want it to start with %q", setting, got, want)
		}
	}
}
