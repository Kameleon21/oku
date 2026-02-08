package cli

import (
	"fmt"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var page string
	var bookID int

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update reading progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			if page == "" {
				return fmt.Errorf("--page is required (e.g. 123, +10, -5)")
			}
			pageUpdate, err := model.ParsePage(page)
			if err != nil {
				return err
			}

			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			newPage, err := a.UpdateProgress(ctx(), bookID, pageUpdate)
			if err != nil {
				return err
			}
			fmt.Printf("Progress updated to page %s\n", pageStyle.Render(fmt.Sprintf("%d", newPage)))
			return nil
		},
	}
	cmd.Flags().StringVar(&page, "page", "", "Page number (123, +10, -5)")
	cmd.Flags().IntVar(&bookID, "book", 0, "Book ID (defaults when there is exactly one active book)")
	return cmd
}
