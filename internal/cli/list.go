package cli

import (
	"fmt"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var refresh bool

	cmd := &cobra.Command{
		Use:   "list <reading|oku|finished|dnf>",
		Short: "List books by status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			status, err := model.StatusFromString(args[0])
			if err != nil {
				return err
			}
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			books, err := a.ListBooks(ctx(), status, refresh)
			if err != nil {
				return err
			}
			printBooks(books)
			return nil
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force refresh from API")
	return cmd
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
				fmt.Println(statusStyle.Render("Currently Reading"))
				fmt.Println()
			}
			printBooks(books)
			return nil
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
			printBooks(books)
			return nil
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Force refresh from API")
	return cmd
}
