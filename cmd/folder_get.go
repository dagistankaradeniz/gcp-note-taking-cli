package cmd

import (
	"fmt"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var folderGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Fetch a single folder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var folder client.Folder
		if err := c.Do("GET", "/v1/folders/"+args[0], nil, nil, &folder); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(folder)
		}
		fmt.Printf("%s (%s)\n", folder.Name, folder.ID)
		return nil
	},
}

func init() {
	folderCmd.AddCommand(folderGetCmd)
}
