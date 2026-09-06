package cli

import (
	"fmt"
	"strings"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var refresh bool

	cmd := &cobra.Command{
		Use:   "list [reading|oku|finished|paused|dnf|ignored]",
		Short: "List books by status (defaults to default_list from config)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			status, err := listStatusFromArgs(args, a.Config.DefaultList)
			if err != nil {
				return err
			}

			books, err := a.ListBooks(ctx(), status, refresh)
			if err != nil {
				return err
			}
			return printBooks(books)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force refresh from API")
	return cmd
}

// listStatusFromArgs resolves the status to list, falling back to the
// configured default_list when no status is given.
func listStatusFromArgs(args []string, defaultList string) (model.Status, error) {
	if len(args) > 0 {
		return model.StatusFromString(args[0])
	}

	if strings.TrimSpace(defaultList) == "" {
		return 0, fmt.Errorf("no status given and default_list is not set (see: oku config edit)")
	}
	status, err := model.StatusFromString(defaultList)
	if err != nil {
		return 0, fmt.Errorf("default_list: %w", err)
	}
	return status, nil
}

func newNowCmd() *cobra.Command {
	var refresh bool

	cmd := &cobra.Command{
		Use:   "now",
		Short: "Show currently reading books",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			books, err := a.ListBooks(ctx(), model.StatusCurrentlyReading, refresh)
			if err != nil {
				return err
			}
			if !jsonOutput && len(books) > 0 {
				outPrintln(statusStyle().Render("Currently Reading"))
				outPrintln()
			}
			return printBooks(books)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force refresh from API")
	return cmd
}

func newShortcutCmd(name, desc string, statusID int) *cobra.Command {
	var refresh bool

	cmd := &cobra.Command{
		Use:   name,
		Short: desc,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			books, err := a.ListBooks(ctx(), model.Status(statusID), refresh)
			if err != nil {
				return err
			}
			return printBooks(books)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force refresh from API")
	return cmd
}
