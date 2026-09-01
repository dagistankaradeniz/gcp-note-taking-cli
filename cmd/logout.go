package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear the locally stored credential",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := storeForCurrent().Delete(); err != nil {
			return fmt.Errorf("clear credential: %w", err)
		}
		fmt.Println("Logged out.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
