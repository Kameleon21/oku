package cli

import (
	"fmt"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/picker"
	"github.com/spf13/cobra"
)

func newOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "Interactively pick a book and add it to active list",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			books, err := a.ListBooks(ctx(), model.StatusCurrentlyReading, false)
			if err != nil {
				return err
			}
			if len(books) == 0 {
				return fmt.Errorf("no currently reading books. Try: oku list oku")
			}

			bookID, err := picker.PickBook(books, "Pick a book")
			if err != nil {
				return err
			}
			if bookID == 0 {
				fmt.Println("Cancelled.")
				return nil
			}

			if err := a.AddActiveBook(bookID); err != nil {
				return err
			}

			// Find the selected book for display.
			for _, b := range books {
				if b.Book.ID == bookID {
					fmt.Printf("Added to active list: %s\n", titleStyle.Render(b.Book.Title))
					break
				}
			}
			return nil
		},
	}
}
