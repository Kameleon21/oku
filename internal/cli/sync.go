package cli

import (
	"fmt"

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
			defer a.Store.Close()

			fmt.Println("Syncing...")
			if err := a.SyncAll(ctx()); err != nil {
				return err
			}
			fmt.Println("Done.")
			return nil
		},
	}
}
