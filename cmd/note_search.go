package cmd

import (
	"net/url"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var noteSearchIncludeSensitive bool

// The v1 API has no server-side search endpoint (see v1_notes.py's module
// docstring -- search stays client-side, same constraint as the browser
// app). `note search` lists notes and filters locally by title/body
// substring match.
var noteSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search notes by title/content (client-side, over your recent notes)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.ToLower(args[0])

		c := newClient()
		var resp client.NoteListResponse
		q := url.Values{}
		q.Set("limit", "200")
		if err := c.Do("GET", "/v1/notes", q, nil, &resp); err != nil {
			return err
		}

		matches := make([]client.Note, 0)
		for _, n := range resp.Notes {
			if strings.Contains(strings.ToLower(n.Title), query) ||
				strings.Contains(strings.ToLower(bodyToText(n.Body)), query) {
				matches = append(matches, n)
			}
		}
		applySensitiveMasking(matches, noteSearchIncludeSensitive)

		if jsonOutput {
			return output.JSON(client.NoteListResponse{Notes: matches, Total: len(matches)})
		}
		printNoteTable(matches)
		return nil
	},
}

func init() {
	noteSearchCmd.Flags().BoolVar(&noteSearchIncludeSensitive, "include-sensitive", false, "include masked/sensitive fields unmasked")
	noteCmd.AddCommand(noteSearchCmd)
}
