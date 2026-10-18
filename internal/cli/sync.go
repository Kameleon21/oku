package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/Kameleon21/oku/internal/app"

	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Refresh all book lists from Hardcover",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Close()

			fmt.Println("Syncing...")
			if err := a.SyncAll(ctx()); errors.Is(err, app.ErrStatsIncomplete) {
				fmt.Fprintln(os.Stderr, "warning:", err)
			} else if err != nil {
				return err
			}
			fmt.Println("Done.")
			return nil
		},
	}
}
