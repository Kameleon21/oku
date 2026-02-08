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
	"github.com/spf13/cobra"
)

var jsonOutput bool

func newRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "oku",
		Short:   "A fast CLI for Hardcover",
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isInteractiveTerminal(os.Stdin) || !isInteractiveTerminal(os.Stdout) {
				return cmd.Help()
			}
			return runDashboard()
		},
	}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

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
	cmd.AddCommand(newTUICmd())
	cmd.AddCommand(newUpdateCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newSyncCmd())

	// Convenience shortcuts.
	cmd.AddCommand(newShortcutCmd("reading", "List currently reading books", 2))
	cmd.AddCommand(newShortcutCmd("oku", "List want-to-read books", 1))
	cmd.AddCommand(newShortcutCmd("finished", "List finished books", 3))
	cmd.AddCommand(newShortcutCmd("dnf", "List did-not-finish books", 5))

	return cmd
}

// Execute runs the root command and returns an exit code.
func Execute(version string) int {
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

	db, err := store.New(config.DBPath())
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	client := api.NewClient(token)
	return app.New(client, db, cfg), nil
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
