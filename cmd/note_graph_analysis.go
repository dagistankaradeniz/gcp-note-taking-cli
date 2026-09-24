package cmd

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	noteGraphAnalysisCommunityAlgo  string
	noteGraphAnalysisCentralityAlgo string
)

// Second Brain (Pro plan): community detection + importance ranking over
// the whole-account graph. community_algo: louvain (default),
// label_propagation, greedy_modularity, connected_components.
// centrality_algo: pagerank (default), betweenness, degree, closeness.
var noteGraphAnalysisCmd = &cobra.Command{
	Use:   "graph-analysis",
	Short: "Run community detection + centrality ranking over your whole-account graph (Pro plan)",
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		if noteGraphAnalysisCommunityAlgo != "" {
			q.Set("community_algo", noteGraphAnalysisCommunityAlgo)
		}
		if noteGraphAnalysisCentralityAlgo != "" {
			q.Set("centrality_algo", noteGraphAnalysisCentralityAlgo)
		}

		c := newClient()
		var resp client.GraphAnalysisResponse
		if err := c.Do("GET", "/v1/notes/graph/global/analysis", q, nil, &resp); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp)
		}
		if resp.ZKUnavailable {
			output.Table([]string{"NOTE"}, [][]string{{"unavailable for Zero-Knowledge accounts"}})
			return nil
		}
		if resp.CentralityApproximated || resp.CommunityFallbackUsed {
			fmt.Println("Note: graph too large for the exact algorithm, a cheaper approximation was used.")
		}
		rows := make([][]string, 0, len(resp.Nodes))
		for _, n := range resp.Nodes {
			rows = append(rows, []string{n.ID, n.Type, strconv.Itoa(n.CommunityID), strconv.FormatFloat(n.Centrality, 'f', 4, 64)})
		}
		output.Table([]string{"ID", "TYPE", "COMMUNITY", "CENTRALITY"}, rows)
		return nil
	},
}

func init() {
	noteGraphAnalysisCmd.Flags().StringVar(&noteGraphAnalysisCommunityAlgo, "community-algo", "louvain", "louvain | label_propagation | greedy_modularity | connected_components")
	noteGraphAnalysisCmd.Flags().StringVar(&noteGraphAnalysisCentralityAlgo, "centrality-algo", "pagerank", "pagerank | betweenness | degree | closeness")
	noteCmd.AddCommand(noteGraphAnalysisCmd)
}
