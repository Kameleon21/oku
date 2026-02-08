package config

import (
	"os"
	"path/filepath"

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
	path := FilePath()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// FilePath returns the config file path (~/.config/oku/config.toml).
func FilePath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "oku", "config.toml")
}

// DataDir returns the data directory (~/.local/share/oku).
func DataDir() string {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "oku")
}

// DBPath returns the SQLite database path.
func DBPath() string {
	return filepath.Join(DataDir(), "cache.db")
}

// EnsureDataDir creates the data directory if it doesn't exist.
func EnsureDataDir() error {
	return os.MkdirAll(DataDir(), 0o755)
}

// EnsureConfigDir creates the config directory if it doesn't exist.
func EnsureConfigDir() error {
	return os.MkdirAll(filepath.Dir(FilePath()), 0o755)
}
