package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Kameleon21/oku/internal/config"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}
	cmd.AddCommand(newConfigEditCmd())
	cmd.AddCommand(newConfigShowCmd())
	return cmd
}

func newConfigEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open config file in your editor",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.EnsureConfigDir(); err != nil {
				return err
			}

			path, err := config.FilePath()
			if err != nil {
				return err
			}

			// Create the file if it doesn't exist.
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err := os.WriteFile(path, []byte("# Oku config\n# editor = \"nvim\"\n# use_fzf = false\n# default_list = \"reading\"\n"), 0o644); err != nil {
					return err
				}
			}

			// A broken config must still be editable, so fall back to defaults
			// instead of refusing to open the file that needs fixing.
			cfg, err := config.Load()
			if err != nil {
				cfg = config.Defaults()
			}

			c := exec.Command(resolveEditor(cfg.Editor, os.Getenv), path)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

// resolveEditor picks the editor to open the config with, preferring the
// configured one, then $VISUAL, then $EDITOR, then vi.
func resolveEditor(cfgEditor string, getenv func(string) string) string {
	for _, candidate := range []string{cfgEditor, getenv("VISUAL"), getenv("EDITOR")} {
		if editor := strings.TrimSpace(candidate); editor != "" {
			return editor
		}
	}
	return "vi"
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			path, err := config.FilePath()
			if err != nil {
				return err
			}
			dataDir, err := config.DataDir()
			if err != nil {
				return err
			}
			fmt.Printf("Config file: %s\n", path)
			fmt.Printf("Data dir:    %s\n", dataDir)
			fmt.Printf("Editor:      %s\n", resolveEditor(cfg.Editor, os.Getenv))
			fmt.Printf("Use fzf:     %v\n", cfg.UseFzf)
			fmt.Printf("Default list: %s\n", cfg.DefaultList)
			return nil
		},
	}
}
