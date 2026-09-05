package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

// Config holds the application configuration.
type Config struct {
	Editor      string `toml:"editor"`
	UseFzf      bool   `toml:"use_fzf"`
	DefaultList string `toml:"default_list"`
	// Theme pins the TUI palette to a "dark" or "light" terminal background;
	// "auto" (the default) lets the terminal report it.
	Theme string `toml:"theme"`
}

// Defaults returns a Config with sensible defaults.
func Defaults() Config {
	return Config{
		Editor:      "",
		UseFzf:      false,
		DefaultList: "reading",
		Theme:       "auto",
	}
}

// Load reads config from the TOML file, falling back to defaults.
func Load() (Config, error) {
	cfg := Defaults()
	path, err := FilePath()
	if err != nil {
		return cfg, err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// pathEnv carries the lookups path resolution depends on, so the rules can be
// unit-tested without touching the real environment.
type pathEnv struct {
	goos      string
	getenv    func(string) string
	home      func() (string, error)
	configDir func() (string, error) // %AppData% on Windows
	cacheDir  func() (string, error) // %LocalAppData% on Windows
	exists    func(string) bool
}

func osPathEnv() pathEnv {
	return pathEnv{
		goos:      runtime.GOOS,
		getenv:    os.Getenv,
		home:      os.UserHomeDir,
		configDir: os.UserConfigDir,
		cacheDir:  os.UserCacheDir,
		exists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
	}
}

// FilePath returns the config file path (~/.config/oku/config.toml on
// macOS and Linux, %AppData%\oku\config.toml on Windows).
func FilePath() (string, error) {
	return configFilePath(osPathEnv())
}

func configFilePath(env pathEnv) (string, error) {
	if dir := env.getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "oku", "config.toml"), nil
	}

	legacy, legacyErr := homeJoin(env, ".config", "oku", "config.toml")
	if env.goos != "windows" {
		if legacyErr != nil {
			return "", fmt.Errorf("locate config dir: %w", legacyErr)
		}
		return legacy, nil
	}

	dir, err := env.configDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	current := filepath.Join(dir, "oku", "config.toml")
	return preferLegacyWindowsPath(env, legacy, legacyErr, current), nil
}

// DataDir returns the data directory (~/.local/share/oku on macOS and Linux,
// %LocalAppData%\oku on Windows).
func DataDir() (string, error) {
	return dataDir(osPathEnv())
}

func dataDir(env pathEnv) (string, error) {
	if dir := env.getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "oku"), nil
	}

	legacy, legacyErr := homeJoin(env, ".local", "share", "oku")
	if env.goos != "windows" {
		if legacyErr != nil {
			return "", fmt.Errorf("locate data dir: %w", legacyErr)
		}
		return legacy, nil
	}

	dir, err := env.cacheDir()
	if err != nil {
		return "", fmt.Errorf("locate data dir: %w", err)
	}
	current := filepath.Join(dir, "oku")
	return preferLegacyWindowsPath(env, legacy, legacyErr, current), nil
}

// preferLegacyWindowsPath keeps Windows installs from before the switch to
// %AppData%/%LocalAppData% pointed at the data they already have: the cache
// holds local-only timer sessions, journals and active-book state that no
// re-sync can recreate. Only used when the new location is still absent.
func preferLegacyWindowsPath(env pathEnv, legacy string, legacyErr error, current string) string {
	if legacyErr == nil && env.exists(legacy) && !env.exists(current) {
		return legacy
	}
	return current
}

func homeJoin(env pathEnv, parts ...string) (string, error) {
	home, err := env.home()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{home}, parts...)...), nil
}

// DBPath returns the SQLite database path.
func DBPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cache.db"), nil
}

// EnsureDataDir creates the data directory if it doesn't exist.
func EnsureDataDir() error {
	dir, err := DataDir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}

// EnsureConfigDir creates the config directory if it doesn't exist.
func EnsureConfigDir() error {
	path, err := FilePath()
	if err != nil {
		return err
	}
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
