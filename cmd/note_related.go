package cmd

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var noteRelatedLimit int

// Second Brain (Pro plan): tag/link-similarity "you might want to link
// these" suggestions.
var noteRelatedCmd = &cobra.Command{
	Use:   "related <id>",
	Short: "Suggest notes related to this note by shared tags/links (Pro plan)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(noteRelatedLimit))

		c := newClient()
		var resp client.NoteRelatedResponse
		if err := c.Do("GET", "/v1/notes/"+args[0]+"/related", q, nil, &resp); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp)
		}
		if resp.ZKUnavailable {
			output.Table([]string{"NOTE"}, [][]string{{"unavailable for Zero-Knowledge accounts"}})
			return nil
		}
		rows := make([][]string, 0, len(resp.Notes))
		for _, n := range resp.Notes {
			rows = append(rows, []string{n.ID, n.Title, n.Type, strconv.FormatFloat(n.Score, 'f', 2, 64), strings.Join(n.SharedTags, ",")})
		}
		output.Table([]string{"ID", "TITLE", "TYPE", "SCORE", "SHARED TAGS"}, rows)
		return nil
	},
}

func init() {
	noteRelatedCmd.Flags().IntVar(&noteRelatedLimit, "limit", 5, "max related notes to return")
	noteCmd.AddCommand(noteRelatedCmd)
}
