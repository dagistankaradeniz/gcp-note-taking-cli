package cmd

import (
	"fmt"
	"os"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/auth"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/browser"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Sign in via the browser (OAuth 2.0 Device Authorization Grant)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		token, err := auth.Login(c, resolveClientID(), shouldOpenBrowser(), func(format string, a ...any) {
			fmt.Printf(format, a...)
		})
		if err != nil {
			return err
		}
		if err := storeForCurrent().Save(token); err != nil {
			return fmt.Errorf("save credential: %w", err)
		}
		fmt.Println("Logged in.")
		return nil
	},
}

// noBrowser keeps `login` and `workspace login` from opening the browser.
var noBrowser bool

// shouldOpenBrowser: on unless --no-browser or $QUILLINK_NO_BROWSER is
// set, and only where a browser can open (not over SSH or headless).
func shouldOpenBrowser() bool {
	return !noBrowser && os.Getenv("QUILLINK_NO_BROWSER") == "" && browser.Available()
}

func init() {
	loginCmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the sign-in link instead of opening the browser")
	rootCmd.AddCommand(loginCmd)
}
