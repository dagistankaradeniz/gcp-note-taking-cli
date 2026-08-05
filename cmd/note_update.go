package cmd

import (
	"fmt"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	noteUpdateTitle    string
	noteUpdateContent  string
	noteUpdateBodyJSON string
	noteUpdateFolderID string
	noteUpdateTags     []string
	noteUpdatePinned   bool
)

var noteUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a note's title, content, folder, or tags",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		req := client.NoteUpdateRequest{}
		flags := cmd.Flags()

		if flags.Changed("title") {
			req.Title = &noteUpdateTitle
		}
		if flags.Changed("body-json") {
			body, err := bodyFromJSON(noteUpdateBodyJSON)
			if err != nil {
				return err
			}
			req.Body = body
		} else if flags.Changed("content") {
			req.Body = textToBody(noteUpdateContent)
		}
		if flags.Changed("folder") {
			req.FolderID = &noteUpdateFolderID
		}
		if flags.Changed("tags") {
			req.Tags = &noteUpdateTags
		}
		if flags.Changed("pinned") {
			req.Pinned = &noteUpdatePinned
		}

		c := newClient()
		var note client.Note
		if err := c.Do("PUT", "/v1/notes/"+args[0], nil, req, &note); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(note)
		}
		fmt.Printf("Updated note %s: %s\n", note.ID, note.Title)
		return nil
	},
}

func init() {
	noteUpdateCmd.Flags().StringVar(&noteUpdateTitle, "title", "", "new title")
	noteUpdateCmd.Flags().StringVar(&noteUpdateContent, "content", "", "new body as plain text")
	noteUpdateCmd.Flags().StringVar(&noteUpdateBodyJSON, "body-json", "", "new body as raw Tiptap JSON (overrides --content)")
	noteUpdateCmd.Flags().StringVar(&noteUpdateFolderID, "folder", "", "move to folder ID")
	noteUpdateCmd.Flags().StringSliceVar(&noteUpdateTags, "tags", nil, "replace tags (comma-separated)")
	noteUpdateCmd.Flags().BoolVar(&noteUpdatePinned, "pinned", false, "pin/unpin the note")
	noteCmd.AddCommand(noteUpdateCmd)
}
