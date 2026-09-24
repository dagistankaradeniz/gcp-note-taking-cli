package cmd

import (
	"net/url"
	"strconv"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var noteGraphHops int

// Second Brain (Pro plan): local link graph centered on one note.
var noteGraphCmd = &cobra.Command{
	Use:   "graph <id>",
	Short: "Show the local link graph centered on this note (Pro plan)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		q.Set("hops", strconv.Itoa(noteGraphHops))

		c := newClient()
		var resp client.NoteGraphResponse
		if err := c.Do("GET", "/v1/notes/"+args[0]+"/graph", q, nil, &resp); err != nil {
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
		return nil
	},
}

func init() {
	noteGraphCmd.Flags().IntVar(&noteGraphHops, "hops", 1, "1 = direct neighbors only, 2 = also expand each neighbor one level further")
	noteCmd.AddCommand(noteGraphCmd)
}

func printGraphNodeTable(nodes []client.NoteGraphNode) {
	rows := make([][]string, 0, len(nodes))
	for _, n := range nodes {
		locked := "no"
		if n.Locked {
			locked = "yes"
		}
		rows = append(rows, []string{n.ID, n.Title, n.Type, locked})
	}
	output.Table([]string{"ID", "TITLE", "TYPE", "LOCKED"}, rows)
}

func printGraphEdgeTable(edges []client.NoteGraphEdge) {
	rows := make([][]string, 0, len(edges))
	for _, e := range edges {
		rows = append(rows, []string{e.Source, e.Target})
	}
	output.Table([]string{"SOURCE", "TARGET"}, rows)
}
