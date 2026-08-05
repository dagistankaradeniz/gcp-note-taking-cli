package cmd

import (
	"fmt"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	noteCreateContent  string
	noteCreateBodyJSON string
	noteCreateFolderID string
	noteCreateTags     []string
	noteCreatePinned   bool
)

var noteCreateCmd = &cobra.Command{
	Use:   "create <title>",
	Short: "Create a note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		req := client.NoteCreateRequest{
			Title:      args[0],
			Tags:       noteCreateTags,
			Pinned:     noteCreatePinned,
			EditorMode: "basic",
		}
		if noteCreateBodyJSON != "" {
			body, err := bodyFromJSON(noteCreateBodyJSON)
			if err != nil {
				return err
			}
			req.Body = body
		} else if noteCreateContent != "" {
			req.Body = textToBody(noteCreateContent)
		}
		if noteCreateFolderID != "" {
			req.FolderID = &noteCreateFolderID
		}

		c := newClient()
		var note client.Note
		if err := c.Do("POST", "/v1/notes", nil, req, &note); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(note)
		}
		fmt.Printf("Created note %s: %s\n", note.ID, note.Title)
		return nil
	},
}

func init() {
	noteCreateCmd.Flags().StringVar(&noteCreateContent, "content", "", "note body as plain text")
	noteCreateCmd.Flags().StringVar(&noteCreateBodyJSON, "body-json", "", "note body as raw Tiptap JSON (overrides --content)")
	noteCreateCmd.Flags().StringVar(&noteCreateFolderID, "folder", "", "folder ID to create the note in")
	noteCreateCmd.Flags().StringSliceVar(&noteCreateTags, "tags", nil, "comma-separated tags")
	noteCreateCmd.Flags().BoolVar(&noteCreatePinned, "pinned", false, "pin the note")
	noteCmd.AddCommand(noteCreateCmd)
}
