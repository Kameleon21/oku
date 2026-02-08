package cli

import (
	"fmt"

	"github.com/Kameleon21/oku/internal/auth"
	"github.com/spf13/cobra"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}
	cmd.AddCommand(newSetTokenCmd())
	return cmd
}

func newSetTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-token",
		Short: "Store your Hardcover API token in the system keychain",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := auth.PromptToken()
			if err != nil {
				return err
			}
			if err := auth.SetToken(token); err != nil {
				return fmt.Errorf("failed to store token: %w", err)
			}
			fmt.Println("Token stored successfully.")
			return nil
		},
	}
}
