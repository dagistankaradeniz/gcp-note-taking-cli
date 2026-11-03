package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/progress"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
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
		if richTable() {
			fmt.Print(ui.Card("Notes", []ui.Field{
				{Label: "Total", Value: progress.Count(stats.TotalNotes)},
				{Label: "Pinned", Value: progress.Count(stats.PinnedNotes)},
				{Label: "Storage", Value: progress.Size(stats.TotalSizeBytes)},
			}))
			printActivity(stats.ByDay)
			return nil
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

// printActivity is notes per day as a small bar chart on a terminal.
func printActivity(byDay map[string]int) {
	if len(byDay) == 0 {
		return
	}
	days := make([]string, 0, len(byDay))
	most := 0
	for d, n := range byDay {
		days = append(days, d)
		most = max(most, n)
	}
	sort.Strings(days)
	fmt.Println(ui.Render(ui.Fg(ui.Muted, "Activity", true)))
	for _, d := range days {
		n := byDay[d]
		bar := strings.Repeat("█", max(1, n*30/max(most, 1)))
		fmt.Println(ui.Render("  " + ui.Fg(ui.Muted, d, false) + "  " + ui.Fg(ui.Brand, bar, false) + " " + fmt.Sprint(n)))
	}
}

func init() {
	noteCmd.AddCommand(noteStatsCmd)
}
