// Package cmd implements the `quillink` Cobra command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/auth"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
	apiBase    string
	tokenFlag  string
	clientID   string
)

var rootCmd = &cobra.Command{
	Use:           "quillink",
	Short:         "Quillink CLI -- command-line access to your Quillink notes",
	Version:       client.Version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		exitCode := output.ExitGeneral
		if apiErr, ok := err.(*client.APIError); ok {
			exitCode = output.CodeForStatus(apiErr.Status)
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(exitCode)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output as stable, versioned JSON instead of a human-readable table")
	rootCmd.PersistentFlags().StringVar(&apiBase, "api-base", "", "override the API base URL (default: "+client.DefaultAPIBase+", or $QUILLINK_API_BASE)")
	rootCmd.PersistentFlags().StringVar(&tokenFlag, "token", "", "credential to use for this request (default: $QUILLINK_TOKEN, or the stored login)")
	rootCmd.PersistentFlags().StringVar(&clientID, "client-id", "", "override the OAuth client_id used by login (default: $QUILLINK_CLIENT_ID, or the value baked in at build time)")
}

// resolveAPIBase applies the --api-base flag / QUILLINK_API_BASE env var /
// built-in default, in that precedence order.
func resolveAPIBase() string {
	if apiBase != "" {
		return apiBase
	}
	if env := os.Getenv("QUILLINK_API_BASE"); env != "" {
		return env
	}
	return client.DefaultAPIBase
}

// resolveToken applies the --token flag / QUILLINK_TOKEN env var / stored
// login credential, in that precedence order (see CLI Access Confluence
// page, "Authentication" and "Local credential storage").
func resolveToken() string {
	if tokenFlag != "" {
		return tokenFlag
	}
	if env := os.Getenv("QUILLINK_TOKEN"); env != "" {
		return env
	}
	token, err := auth.NewStore().Load()
	if err != nil {
		return ""
	}
	return token
}

func newClient() *client.Client {
	return client.New(resolveAPIBase(), resolveToken())
}

// resolveClientID applies the --client-id flag / QUILLINK_CLIENT_ID env
// var / build-time default (client.CLIClientID, injected via -ldflags for
// release builds -- see .goreleaser.yaml), in that precedence order. A
// release binary is baked with one environment's client_id (prod); this
// override is how `--api-base` can point the same binary at a different
// environment (e.g. staging) during testing.
func resolveClientID() string {
	if clientID != "" {
		return clientID
	}
	if env := os.Getenv("QUILLINK_CLIENT_ID"); env != "" {
		return env
	}
	return client.CLIClientID
}
