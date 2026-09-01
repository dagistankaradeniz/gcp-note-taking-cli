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
	noteListFolderID         string
	noteListLimit            int
	noteListStartAfter       string
	noteListIncludeSensitive bool
)

var noteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List notes",
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		if noteListFolderID != "" {
			q.Set("folder_id", noteListFolderID)
		}
		q.Set("limit", strconv.Itoa(noteListLimit))
		if noteListStartAfter != "" {
			q.Set("start_after", noteListStartAfter)
		}

		c := newClient()
		var resp client.NoteListResponse
		if err := c.Do("GET", "/v1/notes", q, nil, &resp); err != nil {
			return err
		}
		applySensitiveMasking(resp.Notes, noteListIncludeSensitive)

		if jsonOutput {
			return output.JSON(resp)
		}
		printNoteTable(resp.Notes)
		if resp.HasMore && len(resp.Notes) > 0 {
			last := resp.Notes[len(resp.Notes)-1]
			fmt.Fprintf(os.Stderr, "\nMore notes available. Continue with:\n  quillink note list --start-after %s\n", last.ID)
		}
		return nil
	},
}

func init() {
	noteListCmd.Flags().StringVar(&noteListFolderID, "folder", "", "filter by folder ID")
	noteListCmd.Flags().IntVar(&noteListLimit, "limit", 50, "max notes to return")
	noteListCmd.Flags().StringVar(&noteListStartAfter, "start-after", "", "resume listing after this note ID (see has_more/next page hint)")
	noteListCmd.Flags().BoolVar(&noteListIncludeSensitive, "include-sensitive", false, "include masked/sensitive fields unmasked")
	noteCmd.AddCommand(noteListCmd)
}

func printNoteTable(notes []client.Note) {
	rows := make([][]string, 0, len(notes))
	for _, n := range notes {
		folder := "-"
		if n.FolderID != nil {
			folder = *n.FolderID
		}
		rows = append(rows, []string{n.ID, n.Title, folder, n.UpdatedAt})
	}
	output.Table([]string{"ID", "TITLE", "FOLDER", "UPDATED"}, rows)
}

// applySensitiveMasking is a placeholder for the CLI-side counterpart of
// the web app's sensitive-text masking (see CLI Access Confluence page,
// "Sensitive/hidden fields"). Note content masking happens server-side
// today (the API itself doesn't return sensitive spans unless
// include_sensitive is set) -- this hook exists so a future client-side
// masking pass has one place to live rather than being scattered across
// list/get/search.
func applySensitiveMasking(_ []client.Note, _ bool) {}
