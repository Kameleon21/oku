package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds the application configuration.
type Config struct {
	Editor      string `toml:"editor"`
	UseFzf      bool   `toml:"use_fzf"`
	DefaultList string `toml:"default_list"`
	// Theme picks the palette the TUI and the coloured CLI output draw with:
	// "auto" (the default) lets the terminal report its background, "dark"
	// and "light" pin it, and a named palette ("nord", "dracula", …) replaces
	// the built-in one. `oku config theme` lists them.
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

// SetTheme writes the `theme` key to the config file. The file is rewritten
// line by line rather than re-encoded from a Config, so a hand-written config
// keeps its comments, its key order and any key this build does not know
// about. name is one of the values the TUI accepts, validated by the caller.
func SetTheme(name string) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}
	path, err := FilePath()
	if err != nil {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return writeFileAtomic(path, []byte(setKey(string(existing), "theme", name)))
}

// writeFileAtomic replaces a file by writing its successor beside it and
// renaming over it, so an interrupted write cannot leave the config
// truncated. The original's mode is kept; a file that did not exist yet is
// created with the same 0o644 a config written by hand would have.
func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// setKey replaces the value of a top-level key in a TOML document, or appends
// the key when the document does not set it. Only the config's own flat
// `key = "value"` shape is handled, which is the whole of oku's config: a key
// inside a table would need the table tracked, and there are none.
func setKey(doc, key, value string) string {
	quoted := fmt.Sprintf("%q", value)

	lines := strings.Split(doc, "\n")
	for i, line := range lines {
		// A commented-out key is left alone: it is documentation, and the
		// real assignment goes after it.
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		name, _, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		// Only the value token is replaced, so the line's own spacing and
		// anything after the value — an inline `# comment`, typically —
		// survive the rewrite.
		start := len(name) + len("=")
		for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
			start++
		}
		lines[i] = line[:start] + quoted + line[valueEnd(line, start):]
		return strings.Join(lines, "\n")
	}

	out := strings.TrimRight(doc, "\n")
	if out != "" {
		out += "\n"
	}
	return out + key + " = " + quoted + "\n"
}

// valueEnd is the index just past the value that starts at start: the closing
// quote of a quoted one, or the run up to an inline comment for a bare one.
func valueEnd(line string, start int) int {
	if start < len(line) && (line[start] == '"' || line[start] == '\'') {
		quote := line[start]
		for i := start + 1; i < len(line); i++ {
			// Only a basic string escapes its quote; a literal one cannot
			// contain the quote at all.
			if line[i] == '\\' && quote == '"' {
				i++
				continue
			}
			if line[i] == quote {
				return i + 1
			}
		}
		return len(line)
	}
	rest := line[start:]
	if hash := strings.IndexByte(rest, '#'); hash >= 0 {
		rest = rest[:hash]
	}
	return start + len(strings.TrimRight(rest, " \t"))
}
