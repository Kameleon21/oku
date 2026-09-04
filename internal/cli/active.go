package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newActiveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "active",
		Short: "Show active books",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			books, err := a.GetActiveBooks()
			if err != nil {
				return err
			}
			printActiveBooks(books)
			return nil
		},
	}
}

func newSetActiveCmd() *cobra.Command {
	var bookID int

	cmd := &cobra.Command{
		Use:   "set-active",
		Short: "Add a book to the active list by ID",
		RunE: func(cmd *cobra.Command, args []string) error {
			if bookID <= 0 {
				return fmt.Errorf("--book is required (use a book ID from search or list)")
			}
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			if err := a.AddActiveBook(bookID); err != nil {
				return err
			}
			fmt.Printf("Added book ID %d to active list\n", bookID)
			return nil
		},
	}
	cmd.Flags().IntVar(&bookID, "book", 0, "Book ID to set as active")
	return cmd
}
