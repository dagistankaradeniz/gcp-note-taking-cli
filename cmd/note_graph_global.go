package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	noteGlobalGraphCursor string
	noteGlobalGraphLimit  int
)

// Second Brain (Pro plan): whole-account link graph, paginated.
var noteGlobalGraphCmd = &cobra.Command{
	Use:   "global-graph",
	Short: "Show the whole-account link graph (Pro plan)",
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		if noteGlobalGraphCursor != "" {
			q.Set("cursor", noteGlobalGraphCursor)
		}
		q.Set("limit", strconv.Itoa(noteGlobalGraphLimit))

		c := newClient()
		var resp client.NoteGraphGlobalResponse
		if err := c.Do("GET", "/v1/notes/graph/global", q, nil, &resp); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp)
		}
		if resp.ZKUnavailable {
			output.Table([]string{"NOTE"}, [][]string{{"unavailable for Zero-Knowledge accounts"}})
			return nil
		}
		printGraphNodeTable(resp.Nodes)
		printGraphEdgeTable(resp.Edges)
		if resp.NextCursor != nil {
			fmt.Fprintf(os.Stderr, "\nMore of the graph available. Continue with:\n  quillink note global-graph --cursor %s\n", *resp.NextCursor)
		}
		return nil
	},
}

func init() {
	noteGlobalGraphCmd.Flags().StringVar(&noteGlobalGraphCursor, "cursor", "", "resume from a previous page's next_cursor")
	noteGlobalGraphCmd.Flags().IntVar(&noteGlobalGraphLimit, "limit", 100, "max nodes/edges to return this page")
	noteCmd.AddCommand(noteGlobalGraphCmd)
}
