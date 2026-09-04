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
}

// Defaults returns a Config with sensible defaults.
func Defaults() Config {
	return Config{
		Editor:      "",
		UseFzf:      false,
		DefaultList: "reading",
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

// FilePath returns the config file path (~/.config/oku/config.toml on
// macOS and Linux, %AppData%\oku\config.toml on Windows).
func FilePath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		var err error
		if runtime.GOOS == "windows" {
			dir, err = os.UserConfigDir()
		} else {
			dir, err = homeSubdir(".config")
		}
		if err != nil {
			return "", fmt.Errorf("locate config dir: %w", err)
		}
	}
	return filepath.Join(dir, "oku", "config.toml"), nil
}

// DataDir returns the data directory (~/.local/share/oku on macOS and Linux,
// %LocalAppData%\oku on Windows).
func DataDir() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		var err error
		if runtime.GOOS == "windows" {
			dir, err = os.UserCacheDir()
		} else {
			dir, err = homeSubdir(".local", "share")
		}
		if err != nil {
			return "", fmt.Errorf("locate data dir: %w", err)
		}
	}
	return filepath.Join(dir, "oku"), nil
}

func homeSubdir(parts ...string) (string, error) {
	home, err := os.UserHomeDir()
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
