package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for books on Hardcover",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			results, err := a.SearchBooks(ctx(), query, limit)
			if err != nil {
				return err
			}
			printSearchResults(results)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Max results to return")
	return cmd
}
