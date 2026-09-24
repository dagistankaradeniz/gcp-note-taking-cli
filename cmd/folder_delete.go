package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var folderDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a folder (soft-delete, moves it to trash)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		if err := c.Do("DELETE", "/v1/folders/"+args[0], nil, nil, nil); err != nil {
			return err
		}
		fmt.Printf("Deleted folder %s\n", args[0])
		return nil
	},
}

func init() {
	folderCmd.AddCommand(folderDeleteCmd)
}
