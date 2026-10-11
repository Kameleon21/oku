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

// authBackend is everything the auth commands reach outside the CLI for: the
// keychain, the environment and Hardcover. A variable so tests can swap the
// whole of it for a fake.
var authBackend = struct {
	current func(context.Context) (auth.Credential, error)
	verify  func(context.Context, oauth2.TokenSource) (string, *oauth2.Token, error)
	logout  func(context.Context) (auth.LogoutReport, error)
}{
	current: auth.Current,
	verify:  verifyLogin,
	logout:  auth.Logout,
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Sign out and revoke stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := authBackend.logout(cmd.Context())
			if report.EnvTokenSet {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: HARDCOVER_TOKEN is set; it still overrides the stored login and was not cleared")
			}
			if report.RevokeErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to revoke token with Hardcover: %v\n", report.RevokeErr)
			}
			if report.ManualRevoke() {
				hintManualRevoke(cmd, report)
			}
			if err != nil {
				return fmt.Errorf("failed to clear stored token: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Logged out.")
			return nil
		},
	}
}

// hintManualRevoke tells the user how to revoke the API key oku just forgot.
// hc_pat_ tokens can be revoked from the account page, but legacy JWTs can
// only be invalidated by pasting the key itself, so offer to show it once.
func hintManualRevoke(cmd *cobra.Command, report auth.LogoutReport) {
	out := cmd.OutOrStdout()
	if report.Kind == auth.KindPersonalAPIKey {
		fmt.Fprintln(out, "Hint: API keys have to be revoked manually at https://hardcover.app/account/api")
		return
	}

	fmt.Fprintln(out, "Hint: legacy API keys can only be revoked by pasting the key at https://api.hardcover.app/invalidate_keys/new")
	fmt.Fprint(out, "Show the key now so you can revoke it? It is removed from oku after logout. [y/N] ")
	answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	fmt.Fprintln(out)
	if a := strings.ToLower(strings.TrimSpace(answer)); a == "y" || a == "yes" {
		fmt.Fprintf(out, "Your API key (keep it secret):\n%s\n", report.LegacyKey)
		return
	}
	fmt.Fprintln(out, "Key not shown. It stays valid until it expires unless you revoke it.")
}

// verifyLogin asks Hardcover who src's token belongs to, refreshing it first
// if it has expired. It returns the token in use afterwards, so a refresh
// shows up in the reported expiry.
func verifyLogin(ctx context.Context, src oauth2.TokenSource) (string, *oauth2.Token, error) {
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
			cred, err := authBackend.current(cmd.Context())
			if err != nil {
				return err
			}
			username, token, err := authBackend.verify(cmd.Context(), cred.TokenSource)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Logged in to Hardcover as %s\n", username)
			switch cred.Kind {
			case auth.KindEnv:
				fmt.Fprintln(out, "Using: HARDCOVER_TOKEN environment variable (overrides any stored login)")
			case auth.KindBrowserLogin:
				fmt.Fprintln(out, "Using: browser login")
				if !token.Expiry.IsZero() {
					fmt.Fprintf(out, "Access token expires %s; oku renews it automatically.\n", token.Expiry.Local().Format("2 Jan 2006 15:04"))
				}
			case auth.KindPersonalAPIKey:
				fmt.Fprintln(out, "Using: personal API key (expires on the date you chose at https://hardcover.app/account/api)")
			default:
				fmt.Fprintln(out, "Using: legacy API key")
				fmt.Fprintln(out, "Hint: run `oku auth login` to switch to browser login")
			}
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
