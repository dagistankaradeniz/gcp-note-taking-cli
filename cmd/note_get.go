package cmd

import (
	"fmt"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var noteGetIncludeSensitive bool

var noteGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Fetch a single note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		var note client.Note
		if err := c.Do("GET", "/v1/notes/"+args[0], nil, nil, &note); err != nil {
			return err
		}
		decryptNoteZK(dek, &note)
		applySensitiveMasking([]client.Note{note}, noteGetIncludeSensitive)

		if jsonOutput {
			return output.JSON(note)
		}
		fmt.Printf("%s\n\n%s\n", note.Title, bodyToText(note.Body))
		return nil
	},
}

func init() {
	noteGetCmd.Flags().BoolVar(&noteGetIncludeSensitive, "include-sensitive", false, "include masked/sensitive fields unmasked")
	noteCmd.AddCommand(noteGetCmd)
}
