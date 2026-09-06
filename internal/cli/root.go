package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/auth"
	"github.com/Kameleon21/oku/internal/config"
	"github.com/Kameleon21/oku/internal/store"
	"github.com/Kameleon21/oku/internal/tui"
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
	outputView string
)

func newRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "oku",
		Short:   "A fast CLI for Hardcover",
		Version: version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if _, err := tui.ParseDensity(outputView); err != nil {
				return err
			}
			// The palette is adaptive; the config key only overrides what
			// the terminal reports about its background, for the TUI and
			// the coloured CLI output alike. A config that does not load is
			// left for the command to report (or, for `config edit`, to
			// tolerate), so only the theme value itself is checked here.
			if cfg, err := config.Load(); err == nil {
				return tui.ApplyThemeSetting(cfg.Theme)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isInteractiveTerminal(os.Stdin) || !isInteractiveTerminal(os.Stdout) {
				return cmd.Help()
			}
			return runDashboard(version)
		},
	}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	cmd.PersistentFlags().StringVar(&outputView, "view", "default", "Output density: compact, default, verbose")

	// Auth commands don't need the full app setup.
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newConfigCmd())

	// Commands that need the app get it lazily via initApp.
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newNowCmd())
	cmd.AddCommand(newSearchCmd())
	cmd.AddCommand(newActiveCmd())
	cmd.AddCommand(newSetActiveCmd())
	cmd.AddCommand(newOpenCmd())
	cmd.AddCommand(newTUICmd(version))
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newRateCmd())
	cmd.AddCommand(newReviewCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newSyncCmd())

	// Local-only commands (no auth required).
	cmd.AddCommand(newTimerCmd())
	cmd.AddCommand(newStatsCmd())

	// Convenience shortcuts.
	cmd.AddCommand(newShortcutCmd("reading", "List currently reading books", 2))
	cmd.AddCommand(newShortcutCmd("oku", "List want-to-read books", 1))
	cmd.AddCommand(newShortcutCmd("finished", "List finished books", 3))
	cmd.AddCommand(newShortcutCmd("dnf", "List did-not-finish books", 5))

	return cmd
}

// Execute runs the root command and returns an exit code.
func Execute(version string) int {
	api.Version = version
	cmd := newRootCmd(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		if api.IsNetworkError(err) {
			return 2
		}
		return 1
	}
	return 0
}

// initApp creates the App instance (API client + store + config).
func initApp() (*app.App, error) {
	token, err := auth.GetToken()
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	if err := config.EnsureDataDir(); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return nil, err
	}

	db, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	client := api.NewClient(token)
	return app.New(client, db, cfg), nil
}

// initLocalApp creates an App with only the local store (no auth/API needed).
func initLocalApp() (*app.App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	if err := config.EnsureDataDir(); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return nil, err
	}

	db, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	return app.New(nil, db, cfg), nil
}

// validateCount rejects count flags that are below 1 or above max (max <= 0
// means unbounded), before they reach code that assumes a positive count.
func validateCount(flag string, value, max int) error {
	if value < 1 {
		return fmt.Errorf("--%s must be at least 1 (got %d)", flag, value)
	}
	if max > 0 && value > max {
		return fmt.Errorf("--%s must be at most %d (got %d)", flag, max, value)
	}
	return nil
}

// ctx returns a background context.
func ctx() context.Context {
	return context.Background()
}

func isInteractiveTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
