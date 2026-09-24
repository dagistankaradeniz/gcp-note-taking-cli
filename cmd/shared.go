package cmd

import (
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

// Read-only, deliberately: v1 has no endpoint to create/revoke a share
// yet (see backend's app/routers/v1_shared.py module docstring) -- this
// only lists notes shared *with* the caller. See `note recipients` for
// who a note the caller owns has been shared *to*.
var sharedCmd = &cobra.Command{
	Use:   "shared",
	Short: "View notes shared with you",
}

var sharedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List notes shared with you (received copies), most recent first",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var resp client.SharedNoteListResponse
		if err := c.Do("GET", "/v1/shared", nil, nil, &resp); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp)
		}
		rows := make([][]string, 0, len(resp.Notes))
		for _, n := range resp.Notes {
			rows = append(rows, []string{n.ID, n.Title, n.SharedByEmail, n.SharedAt})
		}
		output.Table([]string{"ID", "TITLE", "SHARED BY", "SHARED AT"}, rows)
		return nil
	},
}

func init() {
	sharedCmd.AddCommand(sharedListCmd)
	rootCmd.AddCommand(sharedCmd)
}
