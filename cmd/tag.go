package cmd

import (
	"fmt"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

// tags aren't a standalone resource -- they're just a field on each note
// doc (see backend's app/routers/v1_tags.py) -- so `tag list` is the only
// subcommand; there's nothing to create/update/delete independent of a
// note's own `tags` field.
var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "View tags used across your notes",
}

var tagListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every tag used across your active notes",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var tags []string
		if err := c.Do("GET", "/v1/tags", nil, nil, &tags); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(tags)
		}
		for _, t := range tags {
			fmt.Println(t)
		}
		return nil
	},
}

func init() {
	tagCmd.AddCommand(tagListCmd)
	rootCmd.AddCommand(tagCmd)
}
