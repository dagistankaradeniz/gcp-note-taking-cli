package cmd

import (
	"fmt"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	folderUpdateName     string
	folderUpdateParentID string
	folderUpdateColor    string
	folderUpdatePinned   bool
	folderUpdateUnpinned bool
)

var folderUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		req := client.FolderUpdateRequest{}
		if folderUpdateName != "" {
			req.Name = &folderUpdateName
		}
		if cmd.Flags().Changed("parent") {
			req.ParentID = &folderUpdateParentID
		}
		if cmd.Flags().Changed("color") {
			req.Color = &folderUpdateColor
		}
		if folderUpdatePinned {
			pinned := true
			req.Pinned = &pinned
		}
		if folderUpdateUnpinned {
			pinned := false
			req.Pinned = &pinned
		}

		c := newClient()
		var folder client.Folder
		if err := c.Do("PUT", "/v1/folders/"+args[0], nil, req, &folder); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(folder)
		}
		fmt.Printf("Updated folder %s: %s\n", folder.ID, folder.Name)
		return nil
	},
}

func init() {
	folderUpdateCmd.Flags().StringVar(&folderUpdateName, "name", "", "new folder name")
	folderUpdateCmd.Flags().StringVar(&folderUpdateParentID, "parent", "", "new parent folder ID (empty string clears it)")
	folderUpdateCmd.Flags().StringVar(&folderUpdateColor, "color", "", "new folder color")
	folderUpdateCmd.Flags().BoolVar(&folderUpdatePinned, "pin", false, "pin the folder")
	folderUpdateCmd.Flags().BoolVar(&folderUpdateUnpinned, "unpin", false, "unpin the folder")
	folderCmd.AddCommand(folderUpdateCmd)
}
