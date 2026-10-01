package cli

import (
	"fmt"

	"github.com/Kameleon21/oku/internal/auth"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
	}
	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newLogoutCmd())
	cmd.AddCommand(newSetTokenCmd())
	return cmd
}

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Sign in to Hardcover via your browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := auth.Login(cmd.Context(), cmd.OutOrStdout())
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}
			if err := auth.SetToken(token); err != nil {
				return fmt.Errorf("failed to store token: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Login successful.")
			return nil
		},
	}
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Sign out and revoke stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Best-effort revoke: a missing/expired token still gets cleared locally.
			if token, err := auth.StoredToken(); err == nil {
				if err := auth.LogOut(cmd.Context(), token); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to revoke token with Hardcover: %v\n", err)
				}
			}
			if err := auth.DeleteToken(); err != nil {
				return fmt.Errorf("failed to clear stored token: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Logged out.")
			return nil
		},
	}
}

func newSetTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-token",
		Short: "Manually store a Hardcover API token (advanced; prefer 'oku auth login')",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Create a token with the scopes oku needs:\n%s\n", auth.NewTokenURL())
			raw, err := auth.PromptToken()
			if err != nil {
				return err
			}
			token := &oauth2.Token{AccessToken: raw, TokenType: "Bearer"}
			if err := auth.SetToken(token); err != nil {
				return fmt.Errorf("failed to store token: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Token stored successfully.")
			return nil
		},
	}
}
