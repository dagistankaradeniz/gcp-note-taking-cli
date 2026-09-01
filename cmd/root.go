// Package cmd implements the `quillink` Cobra command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/auth"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	jsonOutput    bool
	apiBase       string
	tokenFlag     string
	clientID      string
	workspaceFlag string
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
	rootCmd.PersistentFlags().StringVar(&workspaceFlag, "workspace", "", "use this workspace's api-base/client-id/credential for this command (default: $QUILLINK_WORKSPACE, or the current workspace set via `workspace use`)")
}

// resolveWorkspaceName applies the --workspace flag / QUILLINK_WORKSPACE
// env var / the persisted "current" workspace (`quillink workspace use`),
// in that precedence order. An empty result means "no workspace is
// active" -- callers fall back to the legacy default api-base/credential.
func resolveWorkspaceName() string {
	if workspaceFlag != "" {
		return workspaceFlag
	}
	if env := os.Getenv("QUILLINK_WORKSPACE"); env != "" {
		return env
	}
	reg, err := workspace.Load()
	if err != nil {
		return ""
	}
	return reg.Current
}

// resolveWorkspace resolves the active workspace's full definition, if
// any.
func resolveWorkspace() (workspace.Workspace, bool) {
	name := resolveWorkspaceName()
	if name == "" {
		return workspace.Workspace{}, false
	}
	reg, err := workspace.Load()
	if err != nil {
		return workspace.Workspace{}, false
	}
	return reg.Get(name)
}

// storeForCurrent returns the credential Store that `login`/`logout`
// should act on: the active workspace's own slot if one is selected,
// otherwise the legacy single default slot -- unchanged behavior for
// anyone who has never touched `quillink workspace`.
func storeForCurrent() *auth.Store {
	if name := resolveWorkspaceName(); name != "" {
		return auth.NewStoreFor(name)
	}
	return auth.NewStore()
}

// resolveAPIBase applies the --api-base flag / QUILLINK_API_BASE env var /
// the active workspace's api_base / built-in default, in that precedence
// order.
func resolveAPIBase() string {
	if apiBase != "" {
		return apiBase
	}
	if env := os.Getenv("QUILLINK_API_BASE"); env != "" {
		return env
	}
	if w, ok := resolveWorkspace(); ok && w.APIBase != "" {
		return w.APIBase
	}
	return client.DefaultAPIBase
}

// resolveToken applies the --token flag / QUILLINK_TOKEN env var / the
// active workspace's stored credential / the legacy default stored
// credential, in that precedence order (see CLI Access Confluence page,
// "Authentication" and "Local credential storage").
func resolveToken() string {
	if tokenFlag != "" {
		return tokenFlag
	}
	if env := os.Getenv("QUILLINK_TOKEN"); env != "" {
		return env
	}
	token, err := storeForCurrent().Load()
	if err != nil {
		return ""
	}
	return token
}

func newClient() *client.Client {
	return client.New(resolveAPIBase(), resolveToken())
}

// resolveClientID applies the --client-id flag / QUILLINK_CLIENT_ID env
// var / the active workspace's client_id / build-time default
// (client.CLIClientID, injected via -ldflags for release builds -- see
// .goreleaser.yaml), in that precedence order. A release binary is baked
// with one environment's client_id (prod); this override is how a
// workspace (or --api-base directly) can point the same binary at a
// different environment (e.g. staging) during testing.
func resolveClientID() string {
	if clientID != "" {
		return clientID
	}
	if env := os.Getenv("QUILLINK_CLIENT_ID"); env != "" {
		return env
	}
	if w, ok := resolveWorkspace(); ok && w.ClientID != "" {
		return w.ClientID
	}
	return client.CLIClientID
}
