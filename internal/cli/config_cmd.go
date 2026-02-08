package cli

import (
	"fmt"
	"os"
	"os/exec"

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

			path := config.FilePath()

			// Create the file if it doesn't exist.
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err := os.WriteFile(path, []byte("# Oku config\n# editor = \"nvim\"\n# use_fzf = false\n# default_list = \"reading\"\n"), 0o644); err != nil {
					return err
				}
			}

			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}

			c := exec.Command(editor, path)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
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
			fmt.Printf("Config file: %s\n", config.FilePath())
			fmt.Printf("Data dir:    %s\n", config.DataDir())
			fmt.Printf("Editor:      %s\n", cfg.Editor)
			fmt.Printf("Use fzf:     %v\n", cfg.UseFzf)
			fmt.Printf("Default list: %s\n", cfg.DefaultList)
			return nil
		},
	}
}
