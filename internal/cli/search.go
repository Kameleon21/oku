package cli

import (
	"strings"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	var limit int
	var modeRaw string

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search Hardcover (book/author/genre intent)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCount("limit", limit, 0); err != nil {
				return err
			}
			query := strings.Join(args, " ")
			mode, err := model.ParseSearchMode(modeRaw)
			if err != nil {
				return err
			}
			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			results, err := a.SearchBooks(ctx(), query, limit, mode)
			if err != nil {
				return err
			}
			return printSearchResults(results)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Max results to return")
	cmd.Flags().StringVar(&modeRaw, "mode", "book", "Search mode: book, author, genre")
	return cmd
}
