package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

func newRateCmd() *cobra.Command {
	var bookID int

	cmd := &cobra.Command{
		Use:   "rate [title] <rating>",
		Short: "Set a book rating (0 or 0.5 increments up to 5.0)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ratingRaw := strings.TrimSpace(args[len(args)-1])
			rating, err := strconv.ParseFloat(ratingRaw, 64)
			if err != nil {
				return fmt.Errorf("invalid rating %q", ratingRaw)
			}
			if err := model.ValidateRating(rating); err != nil {
				return err
			}

			title := ""
			if len(args) > 1 {
				title = strings.Join(args[:len(args)-1], " ")
			}

			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			resolvedID, err := resolveBookIDForReviewInput(a, title, bookID, "Select a book to rate")
			if err != nil {
				return err
			}

			if err := a.RateBook(ctx(), resolvedID, rating); err != nil {
				return err
			}

			fmt.Printf("Rating updated to %s (%s)\n", pageStyle.Render(fmt.Sprintf("%.1f", rating)), model.StarString(rating))
			return nil
		},
	}
	cmd.Flags().IntVar(&bookID, "book", 0, "Book ID (skips title lookup/picker)")
	return cmd
}
