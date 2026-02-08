package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newActiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "active",
		Short: "Show the active book",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			ub, err := a.GetActiveBook()
			if err != nil {
				return err
			}
			printActiveBook(ub)
			return nil
		},
	}
}

func newSetActiveCmd() *cobra.Command {
	var bookID int

	cmd := &cobra.Command{
		Use:   "set-active",
		Short: "Set the active book by ID",
		RunE: func(cmd *cobra.Command, args []string) error {
			if bookID <= 0 {
				return fmt.Errorf("--book is required (use a book ID from search or list)")
			}
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			if err := a.SetActiveBook(bookID); err != nil {
				return err
			}
			fmt.Printf("Active book set to ID %d\n", bookID)
			return nil
		},
	}
	cmd.Flags().IntVar(&bookID, "book", 0, "Book ID to set as active")
	return cmd
}
