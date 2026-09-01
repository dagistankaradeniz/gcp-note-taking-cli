package cmd

import (
	"fmt"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/spf13/cobra"
)

var noteDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a note (soft-delete, moves it to trash)",
	Long: `Soft-deletes a note. If the note is password-protected, its password is
required first -- deleting a note you can't prove you can read is refused,
even though ownership alone would otherwise be enough.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()

		var note client.Note
		if err := c.Do("GET", "/v1/notes/"+args[0], nil, nil, &note); err != nil {
			return err
		}
		if note.Locked {
			password, err := resolvePassword()
			if err != nil {
				return err
			}
			// Verify-only: the returned body is discarded, this just
			// proves the caller knows the password before letting them
			// delete content they haven't demonstrated access to.
			if _, err := unlockNoteBody(c, args[0], password); err != nil {
				return err
			}
		}

		if err := c.Do("DELETE", "/v1/notes/"+args[0], nil, nil, nil); err != nil {
			return err
		}
		fmt.Printf("Deleted note %s\n", args[0])
		return nil
	},
}

func init() {
	noteCmd.AddCommand(noteDeleteCmd)
}
