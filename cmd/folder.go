package cmd

import (
	"github.com/spf13/cobra"
)

var folderCmd = &cobra.Command{
	Use:   "folder",
	Short: "Manage folders",
}

func init() {
	rootCmd.AddCommand(folderCmd)
}
