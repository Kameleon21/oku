package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/Kameleon21/oku/internal/app"
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
			defer a.Store.Close()

			// A cache-refresh failure still means the change reached
			// Hardcover, so report success and warn instead of failing.
			err = a.ChangeStatus(ctx(), bookID, status)
			if err != nil && !errors.Is(err, app.ErrCacheRefresh) {
				return err
			}
			outPrintf("Status changed to %s\n", statusStyle().Render(status.Label()))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&bookID, "book", 0, "Book ID (defaults when there is exactly one active book)")
	return cmd
}
