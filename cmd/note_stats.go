package cmd

import (
	"fmt"
	"sort"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var noteStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show account-wide note stats (total notes, storage used, pinned count)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var stats client.NoteStatsResponse
		if err := c.Do("GET", "/v1/notes/stats", nil, nil, &stats); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(stats)
		}
		fmt.Printf("Total notes:  %d\n", stats.TotalNotes)
		fmt.Printf("Total size:   %d bytes\n", stats.TotalSizeBytes)
		fmt.Printf("Pinned notes: %d\n", stats.PinnedNotes)
		days := make([]string, 0, len(stats.ByDay))
		for d := range stats.ByDay {
			days = append(days, d)
		}
		sort.Strings(days)
		for _, d := range days {
			fmt.Printf("  %s  %d\n", d, stats.ByDay[d])
		}
		return nil
	},
}

func init() {
	noteCmd.AddCommand(noteStatsCmd)
}
