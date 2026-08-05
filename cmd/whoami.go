package cmd

import (
	"fmt"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show the signed-in account's scopes and credential type",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var status client.StatusResponse
		if err := c.Do("GET", "/v1/status", nil, nil, &status); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(status)
		}
		fmt.Printf("Credential type: %s\n", status.CredentialType)
		fmt.Printf("Scopes: %v\n", status.Scopes)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}
