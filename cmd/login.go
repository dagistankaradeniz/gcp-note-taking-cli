package cmd

import (
	"fmt"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/auth"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Sign in via the browser (OAuth 2.0 Device Authorization Grant)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		token, err := auth.Login(c, resolveClientID(), func(format string, a ...any) {
			fmt.Printf(format, a...)
		})
		if err != nil {
			return err
		}
		if err := auth.NewStore().Save(token); err != nil {
			return fmt.Errorf("save credential: %w", err)
		}
		fmt.Println("Logged in.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
