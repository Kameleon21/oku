package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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
			token, err := authBackend.login(ctx, cmd.OutOrStdout())
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
			recordSignedInUser(cmd)
			return nil
		},
	}
}

// authBackend is everything the auth commands reach outside the CLI for: the
// browser sign-in, the token prompt, the keychain, the environment,
// Hardcover and the local cache. A variable so tests can swap the whole of
// it for a fake.
var authBackend = struct {
	login       func(context.Context, io.Writer) (*oauth2.Token, error)
	promptToken func() (string, error)
	current     func(context.Context) (auth.Credential, error)
	verify      func(context.Context, oauth2.TokenSource) (int, string, *oauth2.Token, error)
	logout      func(context.Context, func(auth.LogoutReport, string)) (auth.LogoutReport, error)
	remember    func(userID int) error
}{
	login:       auth.Login,
	promptToken: auth.PromptToken,
	current:     auth.Current,
	verify:      verifyLogin,
	logout:      auth.Logout,
	remember:    rememberSignedInUser,
}

// recordSignedInUser tells the local cache which Hardcover account is signed
// in now, so a previous account's cached library is dropped rather than
// shown as this one's. It costs one request, the GetMe that confirms the
// login. A failure is only a warning: the login itself worked, and the next
// full sync checks the account again.
func recordSignedInUser(cmd *cobra.Command) {
	ctx := cmd.Context()
	cred, err := authBackend.current(ctx)
	var id int
	if err == nil {
		id, _, _, err = authBackend.verify(ctx, cred.TokenSource)
	}
	if err == nil {
		err = authBackend.remember(id)
	}
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not check which account is signed in: %v\n", err)
	}
}

// rememberSignedInUser records userID in the local cache (see
// App.RememberUser).
func rememberSignedInUser(userID int) error {
	a, err := initLocalApp()
	if err != nil {
		return err
	}
	defer a.Close()
	return a.RememberUser(userID)
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Sign out and revoke stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Everything is told before the keychain is cleared, so aborting
			// at the legacy key prompt leaves the key stored to show again.
			_, err := authBackend.logout(cmd.Context(), func(report auth.LogoutReport, legacyKey string) {
				if report.EnvTokenSet {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: HARDCOVER_TOKEN is set; it still overrides the stored login and was not cleared")
				}
				if report.RevokeErr != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to revoke token with Hardcover: %v\n", report.RevokeErr)
				}
				if report.ManualRevoke() {
					hintManualRevoke(cmd, report.Kind, legacyKey)
				}
			})
			if err != nil {
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
func hintManualRevoke(cmd *cobra.Command, kind auth.Kind, legacyKey string) {
	out := cmd.OutOrStdout()
	if kind == auth.KindPersonalAPIKey {
		fmt.Fprintln(out, "Hint: API keys have to be revoked manually at https://hardcover.app/account/api")
		return
	}

	fmt.Fprintln(out, "Hint: legacy API keys can only be revoked by pasting the key at https://api.hardcover.app/invalidate_keys/new")
	fmt.Fprint(out, "Show the key now so you can revoke it? It is removed from oku after logout. [y/N] ")
	answer, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	fmt.Fprintln(out)
	if a := strings.ToLower(strings.TrimSpace(answer)); a == "y" || a == "yes" {
		fmt.Fprintf(out, "Your API key (keep it secret):\n%s\n", legacyKey)
		return
	}
	fmt.Fprintln(out, "Key not shown. It stays valid until it expires unless you revoke it.")
}

// verifyLogin asks Hardcover who src's token belongs to, refreshing it first
// if it has expired. It returns the token in use afterwards, so a refresh
// shows up in the reported expiry.
func verifyLogin(ctx context.Context, src oauth2.TokenSource) (int, string, *oauth2.Token, error) {
	id, username, err := api.NewOAuthClient(ctx, src).GetMe(ctx)
	if err != nil {
		return 0, "", nil, err
	}
	current, err := src.Token()
	if err != nil {
		return 0, "", nil, err
	}
	return id, username, current, nil
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
			userID, username, token, err := authBackend.verify(cmd.Context(), cred.TokenSource)
			if err != nil {
				return err
			}
			// The account is known now at no extra cost; recording it keeps
			// another account's cache from showing. Status only reports, so
			// a failure to record is left to the next login or sync.
			_ = authBackend.remember(userID)

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
			raw, err := authBackend.promptToken()
			if err != nil {
				return err
			}
			token := &oauth2.Token{AccessToken: raw, TokenType: "Bearer"}
			if err := auth.SetToken(token); err != nil {
				return fmt.Errorf("failed to store token: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Token stored successfully.")
			recordSignedInUser(cmd)
			return nil
		},
	}
}
