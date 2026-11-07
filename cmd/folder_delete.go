package cmd

import (
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
	"github.com/spf13/cobra"
)

var folderDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a folder (soft-delete, moves it to trash)",
	Long: `Soft-deletes a folder, after showing it and asking you to confirm. --force
skips the question, and scripts and CI must pass it.`,
	Example: `  quillink folder delete <id>
  quillink folder delete <id> --force`,
	Annotations: confirmsDelete,
	Args:        cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := canConfirm(); err != nil {
			return err
		}
		c := newClient()
		if !forceFlag {
			dek, err := resolveZkDek(c) // the name is encrypted on a ZK account
			if err != nil {
				return err
			}
			var folder client.Folder
			if err := c.Do("GET", "/v1/folders/"+args[0], nil, nil, &folder); err != nil {
				return err
			}
			decryptFolderZK(dek, &folder)
			ok, err := confirmDelete("folder", []ui.Field{
				{Label: "ID", Value: folder.ID},
				{Label: "Name", Value: orDefault(folder.Name, "Untitled")},
				{Label: "Effect", Value: "moves it to the trash"},
			})
			if err != nil || !ok {
				return err
			}
		}
		if err := c.Do("DELETE", "/v1/folders/"+args[0], nil, nil, nil); err != nil {
			return err
		}
		success("Deleted folder %s", args[0])
		return nil
	},
}

func init() {
	folderCmd.AddCommand(folderDeleteCmd)
}
