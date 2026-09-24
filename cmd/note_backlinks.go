package cmd

import (
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

// Second Brain (Pro plan): notes that link to the given note.
var noteBacklinksCmd = &cobra.Command{
	Use:   "backlinks <id>",
	Short: "List notes that link to this note (Pro plan)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var resp client.NoteBacklinksResponse
		if err := c.Do("GET", "/v1/notes/"+args[0]+"/backlinks", nil, nil, &resp); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp)
		}
		if resp.ZKUnavailable {
			output.Table([]string{"NOTE"}, [][]string{{"unavailable for Zero-Knowledge accounts"}})
			return nil
		}
		rows := make([][]string, 0, len(resp.Backlinks))
		for _, b := range resp.Backlinks {
			rows = append(rows, []string{b.ID, b.Title, b.Snippet})
		}
		output.Table([]string{"ID", "TITLE", "SNIPPET"}, rows)
		return nil
	},
}

func init() {
	noteCmd.AddCommand(noteBacklinksCmd)
}
