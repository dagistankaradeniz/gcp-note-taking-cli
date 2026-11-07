package cmd

import (
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
	"github.com/spf13/cobra"
)

var noteDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a note (soft-delete, moves it to trash)",
	Long: `Soft-deletes a note, after showing it and asking you to confirm. --force
skips the question, and scripts and CI must pass it. If the note is
password-protected, its password is required either way -- deleting a note
you can't prove you can read is refused, even though ownership alone would
otherwise be enough.`,
	Example: `  quillink note delete <id>
  quillink note delete <id> --force`,
	Annotations: confirmsDelete,
	Args:        cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := canConfirm(); err != nil {
			return err
		}
		c := newClient()

		var note client.Note
		if err := c.Do("GET", "/v1/notes/"+args[0], nil, nil, &note); err != nil {
			return err
		}
		if !forceFlag {
			dek, err := resolveZkDek(c) // the title is encrypted on a ZK account
			if err != nil {
				return err
			}
			decryptNoteZK(dek, &note)
		}
		ok, err := confirmDelete("note", []ui.Field{
			{Label: "ID", Value: note.ID},
			{Label: "Title", Value: orDefault(note.Title, "Untitled")},
			{Label: "Updated", Value: ui.Relative(note.UpdatedAt)},
			{Label: "Effect", Value: "moves it to the trash"},
		})
		if err != nil || !ok {
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
		success("Deleted note %s", args[0])
		return nil
	},
}

func init() {
	noteCmd.AddCommand(noteDeleteCmd)
}
