package cmd

import (
	"fmt"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	folderCreateParentID string
	folderCreateColor    string
)

var folderCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		req := client.FolderCreateRequest{Name: args[0]}
		if folderCreateParentID != "" {
			req.ParentID = &folderCreateParentID
		}
		if folderCreateColor != "" {
			req.Color = &folderCreateColor
		}

		c := newClient()
		var folder client.Folder
		if err := c.Do("POST", "/v1/folders", nil, req, &folder); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(folder)
		}
		fmt.Printf("Created folder %s: %s\n", folder.ID, folder.Name)
		return nil
	},
}

func init() {
	folderCreateCmd.Flags().StringVar(&folderCreateParentID, "parent", "", "parent folder ID (for nested folders)")
	folderCreateCmd.Flags().StringVar(&folderCreateColor, "color", "", "folder color")
	folderCmd.AddCommand(folderCreateCmd)
}
