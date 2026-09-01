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
	noteSearchLimit            int
	noteSearchIncludeSensitive bool
)

// GET /v1/notes/search is a real server-side endpoint (v1_notes.py),
// added specifically for API/MCP consumers -- it also correctly excludes
// locked-note bodies from the substring match. Use it directly instead of
// paging through notes and filtering client-side.
var noteSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search notes by title/content (server-side, full account)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var resp client.NoteListResponse
		q := url.Values{}
		q.Set("q", args[0])
		q.Set("limit", strconv.Itoa(noteSearchLimit))
		if err := c.Do("GET", "/v1/notes/search", q, nil, &resp); err != nil {
			return err
		}
		applySensitiveMasking(resp.Notes, noteSearchIncludeSensitive)

		if jsonOutput {
			return output.JSON(resp)
		}
		printNoteTable(resp.Notes)
		if resp.HasMore {
			fmt.Fprintf(os.Stderr, "\nMore matches available. Narrow your query or raise --limit (currently %d).\n", noteSearchLimit)
		}
		return nil
	},
}

func init() {
	noteSearchCmd.Flags().IntVar(&noteSearchLimit, "limit", 50, "max matches to return")
	noteSearchCmd.Flags().BoolVar(&noteSearchIncludeSensitive, "include-sensitive", false, "include masked/sensitive fields unmasked")
	noteCmd.AddCommand(noteSearchCmd)
}
