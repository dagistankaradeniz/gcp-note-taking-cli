package cmd

import (
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var noteRecipientsCmd = &cobra.Command{
	Use:   "recipients <id>",
	Short: "List who this note has been shared with",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var resp client.SharedRecipientListResponse
		if err := c.Do("GET", "/v1/notes/"+args[0]+"/recipients", nil, nil, &resp); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp)
		}
		rows := make([][]string, 0, len(resp.Recipients))
		for _, r := range resp.Recipients {
			rows = append(rows, []string{r.RecipientEmail, r.ShareMode, r.SharedAt})
		}
		output.Table([]string{"EMAIL", "MODE", "SHARED AT"}, rows)
		return nil
	},
}

func init() {
	noteCmd.AddCommand(noteRecipientsCmd)
}
