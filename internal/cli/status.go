package cli

import (
	"fmt"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var bookID int

	cmd := &cobra.Command{
		Use:   "status <reading|oku|finished|dnf>",
		Short: "Change a book's status",
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

			if err := a.ChangeStatus(ctx(), bookID, status); err != nil {
				return err
			}
			fmt.Printf("Status changed to %s\n", statusStyle.Render(status.Label()))
			return nil
		},
	}
	cmd.Flags().IntVar(&bookID, "book", 0, "Book ID (defaults to active book)")
	return cmd
}
