package cmd

import (
	"fmt"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
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
		if richTable() {
			fields := []ui.Field{}
			if status.Email != "" {
				fields = append(fields, ui.Field{Label: "Account", Value: status.Email})
			}
			if status.Plan != "" {
				fields = append(fields, ui.Field{Label: "Plan", Value: planName(status.Plan)})
			}
			fields = append(fields,
				ui.Field{Label: "Signed in with", Value: credentialName(status.CredentialType)},
				ui.Field{Label: "Workspace", Value: workspaceLabel()},
				ui.Field{Label: "Access", Value: strings.Join(status.Scopes, ", ")},
			)
			fmt.Print(ui.Card("Signed in", fields))
			return nil
		}
		fmt.Printf("Credential type: %s\n", status.CredentialType)
		fmt.Printf("Scopes: %v\n", status.Scopes)
		return nil
	},
}

// planName is the plan as the web app names it.
func planName(plan string) string {
	if plan == "" {
		return ""
	}
	return strings.ToUpper(plan[:1]) + plan[1:]
}

func credentialName(kind string) string {
	switch kind {
	case "oauth":
		return "browser sign-in"
	case "pat":
		return "access token"
	}
	return kind
}

// workspaceLabel names where commands go: the workspace, or the default
// sign-in (plus its server when it isn't the usual one).
func workspaceLabel() string {
	name := resolveWorkspaceName()
	if name == "" {
		name = "default"
	}
	if base := resolveAPIBase(); base != client.DefaultAPIBase {
		return name + " (" + base + ")"
	}
	return name
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}
