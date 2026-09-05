package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// testEnv builds a pathEnv whose existence checks hit a real temp directory,
// so the Windows legacy-path rules can be exercised on any host.
func testEnv(goos, home string, env map[string]string) pathEnv {
	return pathEnv{
		goos:      goos,
		getenv:    func(key string) string { return env[key] },
		home:      func() (string, error) { return home, nil },
		configDir: func() (string, error) { return filepath.Join(home, "AppData", "Roaming"), nil },
		cacheDir:  func() (string, error) { return filepath.Join(home, "AppData", "Local"), nil },
		exists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte("# oku\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func TestConfigFilePath(t *testing.T) {
	t.Run("unix uses ~/.config", func(t *testing.T) {
		home := t.TempDir()
		got, err := configFilePath(testEnv("darwin", home, nil))
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".config", "oku", "config.toml")
		if got != want {
			t.Fatalf("configFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("unix ignores a legacy-looking dir on windows rules", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, ".config", "oku", "config.toml"))
		got, err := configFilePath(testEnv("linux", home, nil))
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".config", "oku", "config.toml")
		if got != want {
			t.Fatalf("configFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("XDG_CONFIG_HOME wins", func(t *testing.T) {
		home := t.TempDir()
		xdg := t.TempDir()
		got, err := configFilePath(testEnv("windows", home, map[string]string{"XDG_CONFIG_HOME": xdg}))
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(xdg, "oku", "config.toml")
		if got != want {
			t.Fatalf("configFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("windows fresh install uses AppData", func(t *testing.T) {
		home := t.TempDir()
		got, err := configFilePath(testEnv("windows", home, nil))
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, "AppData", "Roaming", "oku", "config.toml")
		if got != want {
			t.Fatalf("configFilePath() = %q, want %q", got, want)
		}
	})

	t.Run("windows keeps the legacy config when only it exists", func(t *testing.T) {
		home := t.TempDir()
		legacy := filepath.Join(home, ".config", "oku", "config.toml")
		writeFile(t, legacy)
		got, err := configFilePath(testEnv("windows", home, nil))
		if err != nil {
			t.Fatal(err)
		}
		if got != legacy {
			t.Fatalf("configFilePath() = %q, want legacy %q", got, legacy)
		}
	})

	t.Run("windows prefers AppData once it exists", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, ".config", "oku", "config.toml"))
		current := filepath.Join(home, "AppData", "Roaming", "oku", "config.toml")
		writeFile(t, current)
		got, err := configFilePath(testEnv("windows", home, nil))
		if err != nil {
			t.Fatal(err)
		}
		if got != current {
			t.Fatalf("configFilePath() = %q, want %q", got, current)
		}
	})

	t.Run("home lookup failure is reported", func(t *testing.T) {
		env := testEnv("linux", "", nil)
		env.home = func() (string, error) { return "", errors.New("no home") }
		if _, err := configFilePath(env); err == nil {
			t.Fatal("expected an error when the home dir cannot be located")
		}
	})
}

func TestDataDir(t *testing.T) {
	t.Run("unix uses ~/.local/share", func(t *testing.T) {
		home := t.TempDir()
		got, err := dataDir(testEnv("darwin", home, nil))
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".local", "share", "oku")
		if got != want {
			t.Fatalf("dataDir() = %q, want %q", got, want)
		}
	})

	t.Run("XDG_DATA_HOME wins", func(t *testing.T) {
		home := t.TempDir()
		xdg := t.TempDir()
		got, err := dataDir(testEnv("windows", home, map[string]string{"XDG_DATA_HOME": xdg}))
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(xdg, "oku")
		if got != want {
			t.Fatalf("dataDir() = %q, want %q", got, want)
		}
	})

	t.Run("windows fresh install uses LocalAppData", func(t *testing.T) {
		home := t.TempDir()
		got, err := dataDir(testEnv("windows", home, nil))
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, "AppData", "Local", "oku")
		if got != want {
			t.Fatalf("dataDir() = %q, want %q", got, want)
		}
	})

	t.Run("windows keeps the legacy cache when only it exists", func(t *testing.T) {
		home := t.TempDir()
		legacy := filepath.Join(home, ".local", "share", "oku")
		writeFile(t, filepath.Join(legacy, "cache.db"))
		got, err := dataDir(testEnv("windows", home, nil))
		if err != nil {
			t.Fatal(err)
		}
		if got != legacy {
			t.Fatalf("dataDir() = %q, want legacy %q", got, legacy)
		}
	})

	t.Run("windows prefers LocalAppData once it exists", func(t *testing.T) {
		home := t.TempDir()
		writeFile(t, filepath.Join(home, ".local", "share", "oku", "cache.db"))
		current := filepath.Join(home, "AppData", "Local", "oku")
		mkdirAll(t, current)
		got, err := dataDir(testEnv("windows", home, nil))
		if err != nil {
			t.Fatal(err)
		}
		if got != current {
			t.Fatalf("dataDir() = %q, want %q", got, current)
		}
	})

	t.Run("home lookup failure is reported", func(t *testing.T) {
		env := testEnv("linux", "", nil)
		env.home = func() (string, error) { return "", errors.New("no home") }
		if _, err := dataDir(env); err == nil {
			t.Fatal("expected an error when the home dir cannot be located")
		}
	})
}

func TestThemeDefaultsToAutoAndLoadsFromFile(t *testing.T) {
	if got := Defaults().Theme; got != "auto" {
		t.Fatalf("Defaults().Theme = %q, want auto", got)
	}

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	path := filepath.Join(home, "oku", "config.toml")
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte("theme = \"light\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Theme != "light" {
		t.Fatalf("cfg.Theme = %q, want light", cfg.Theme)
	}
	// The other keys keep their defaults when the file leaves them out.
	if cfg.DefaultList != "reading" {
		t.Fatalf("cfg.DefaultList = %q, want the default", cfg.DefaultList)
	}
}
