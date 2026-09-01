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

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage named environment/account profiles",
	Long: `A workspace is a named api-base + optional OAuth client_id + its own
stored credential -- define one per environment (prod/staging/local) or
per account (multiple logins against the same environment) and switch
between them with --workspace, $QUILLINK_WORKSPACE, or "workspace use".

Everyone who never runs this command sees no change: plain "quillink
login"/"note list"/etc. keep using the original single stored credential
until you opt in by adding a workspace.`,
}

var (
	wsAddAPIBase  string
	wsAddClientID string
)

var workspaceAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Define a new workspace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if wsAddAPIBase == "" {
			return fmt.Errorf("--api-base is required")
		}
		reg, err := workspace.Load()
		if err != nil {
			return err
		}
		reg.Workspaces[name] = workspace.Workspace{Name: name, APIBase: wsAddAPIBase, ClientID: wsAddClientID}
		if err := reg.Save(); err != nil {
			return err
		}
		fmt.Printf("Workspace %q added (%s). Run `quillink workspace login %s` to sign in.\n", name, wsAddAPIBase, name)
		return nil
	},
}

var workspaceLoginCmd = &cobra.Command{
	Use:   "login <name>",
	Short: "Attach a credential to a workspace (PAT via --token/$QUILLINK_TOKEN, or OAuth device grant otherwise)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		reg, err := workspace.Load()
		if err != nil {
			return err
		}
		w, ok := reg.Get(name)
		if !ok {
			return fmt.Errorf("unknown workspace %q -- run `quillink workspace add %s --api-base <url>` first", name, name)
		}

		// A PAT (--token or $QUILLINK_TOKEN) skips OAuth entirely --
		// useful for environments like local dev that don't have an
		// OAuth client_id registered yet (OAuth login would otherwise
		// fail with "Unknown client_id").
		token := tokenFlag
		if token == "" {
			token = os.Getenv("QUILLINK_TOKEN")
		}
		if token == "" {
			cid := w.ClientID
			if cid == "" {
				cid = resolveClientID()
			}
			c := client.New(w.APIBase, "")
			token, err = auth.Login(c, cid, func(format string, a ...any) {
				fmt.Printf(format, a...)
			})
			if err != nil {
				return err
			}
		}
		if err := auth.NewStoreFor(name).Save(token); err != nil {
			return fmt.Errorf("save credential: %w", err)
		}
		fmt.Printf("Logged in to workspace %q.\n", name)
		return nil
	},
}

var workspaceUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set the current workspace for future commands",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		reg, err := workspace.Load()
		if err != nil {
			return err
		}
		if _, ok := reg.Get(name); !ok {
			return fmt.Errorf("unknown workspace %q -- run `quillink workspace add %s --api-base <url>` first", name, name)
		}
		reg.Current = name
		if err := reg.Save(); err != nil {
			return err
		}
		fmt.Printf("Current workspace set to %q.\n", name)
		return nil
	},
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := workspace.Load()
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(reg)
		}
		rows := make([][]string, 0, len(reg.Workspaces))
		for name, w := range reg.Workspaces {
			current := ""
			if name == reg.Current {
				current = "*"
			}
			loggedIn := "no"
			if _, err := auth.NewStoreFor(name).Load(); err == nil {
				loggedIn = "yes"
			}
			rows = append(rows, []string{current, name, w.APIBase, loggedIn})
		}
		output.Table([]string{"", "NAME", "API BASE", "LOGGED IN"}, rows)
		return nil
	},
}

var workspaceRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a workspace and forget its stored credential",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		reg, err := workspace.Load()
		if err != nil {
			return err
		}
		delete(reg.Workspaces, name)
		if reg.Current == name {
			reg.Current = ""
		}
		if err := reg.Save(); err != nil {
			return err
		}
		_ = auth.NewStoreFor(name).Delete()
		fmt.Printf("Workspace %q removed.\n", name)
		return nil
	},
}

func init() {
	workspaceAddCmd.Flags().StringVar(&wsAddAPIBase, "api-base", "", "API base URL for this workspace (required)")
	workspaceAddCmd.Flags().StringVar(&wsAddClientID, "client-id", "", "OAuth client_id for this workspace's login (default: global --client-id/$QUILLINK_CLIENT_ID/build-time default)")

	workspaceCmd.AddCommand(workspaceAddCmd, workspaceLoginCmd, workspaceUseCmd, workspaceListCmd, workspaceRemoveCmd)
	rootCmd.AddCommand(workspaceCmd)
}
