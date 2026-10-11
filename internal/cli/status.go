package cli

import (
	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var bookID int

	cmd := &cobra.Command{
		Use:   "status <reading|oku|finished|paused|dnf|ignored>",
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
			defer a.Close()

			err = a.ChangeStatus(ctx(), bookID, status)
			return finishWrite(err, func() {
				outPrintf("Status changed to %s\n", statusStyle().Render(status.Label()))
			})
		},
	}
	cmd.Flags().IntVar(&bookID, "book", 0, "Book ID (defaults when there is exactly one active book)")
	return cmd
}
