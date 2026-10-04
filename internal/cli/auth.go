package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/api"
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
	cmd.AddCommand(newAuthStatusCmd())
	return cmd
}

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Sign in to Hardcover via your browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()
			token, err := auth.Login(ctx, cmd.OutOrStdout())
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					return fmt.Errorf("timed out waiting for browser sign-in")
				}
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
			if auth.EnvTokenSet() {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: HARDCOVER_TOKEN is set; it still overrides the stored login and was not cleared")
			}

			// Best-effort revoke: a missing/expired token still gets cleared locally.
			if token, err := auth.StoredToken(); err == nil {
				if err := auth.LogOut(cmd.Context(), token); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to revoke token with Hardcover: %v\n", err)
				}

				// API keys have no refresh token and can't be revoked via OAuth
				if token.RefreshToken == "" {
					hintManualRevoke(cmd, token.AccessToken)
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

// hintManualRevoke tells the user how to revoke an API key that oku is about
// to forget. hc_pat_ tokens can be revoked from the account page, but legacy
// JWTs can only be invalidated by pasting the key itself, so offer to show it
// before it's deleted from the keychain.
func hintManualRevoke(cmd *cobra.Command, key string) {
	out := cmd.OutOrStdout()
	if isPersonalAccessToken(key) {
		fmt.Fprintln(out, "Hint: API keys have to be revoked manually at https://hardcover.app/account/api")
		return
	}

	fmt.Fprintln(out, "Hint: legacy API keys can only be revoked by pasting the key at https://api.hardcover.app/invalidate_keys/new")
	fmt.Fprint(out, "Show the key now so you can revoke it? It is removed from oku after logout. [y/N] ")
	answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	fmt.Fprintln(out)
	if a := strings.ToLower(strings.TrimSpace(answer)); a == "y" || a == "yes" {
		fmt.Fprintf(out, "Your API key (keep it secret):\n%s\n", key)
		return
	}
	fmt.Fprintln(out, "Key not shown. It stays valid until it expires unless you revoke it.")
}

// verifyLogin asks Hardcover who token belongs to, refreshing it first if it
// has expired. It returns the token in use afterwards, so a refresh shows up
// in the reported expiry. A variable so tests can stub out the network.
var verifyLogin = func(ctx context.Context, token *oauth2.Token) (string, *oauth2.Token, error) {
	src := auth.TokenSource(ctx, token)
	_, username, err := api.NewOAuthClient(ctx, src).GetMe(ctx)
	if err != nil {
		return "", nil, err
	}
	current, err := src.Token()
	if err != nil {
		return "", nil, err
	}
	return username, current, nil
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether you're logged in to Hardcover",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := auth.GetToken()
			if err != nil {
				return err
			}
			username, token, err := verifyLogin(cmd.Context(), token)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Logged in to Hardcover as %s\n", username)
			switch {
			case auth.EnvTokenSet():
				fmt.Fprintln(out, "Using: HARDCOVER_TOKEN environment variable (overrides any stored login)")
			case token.RefreshToken != "":
				fmt.Fprintln(out, "Using: browser login")
				if !token.Expiry.IsZero() {
					fmt.Fprintf(out, "Access token expires %s; oku renews it automatically.\n", token.Expiry.Local().Format("2 Jan 2006 15:04"))
				}
			case isPersonalAccessToken(token.AccessToken):
				fmt.Fprintln(out, "Using: personal API key (expires on the date you chose at https://hardcover.app/account/api)")
			default:
				fmt.Fprintln(out, "Using: legacy API key")
				fmt.Fprintln(out, "Hint: run `oku auth login` to switch to browser login")
			}
			return nil
		},
	}
}

func isPersonalAccessToken(token string) bool {
	return strings.HasPrefix(token, "hc_pat_")
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
