package cmd

import (
	"net/url"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var folderListIncludeTrashed bool

var folderListCmd = &cobra.Command{
	Use:   "list",
	Short: "List folders",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		q := url.Values{}
		if folderListIncludeTrashed {
			q.Set("include_trashed", "true")
		}
		var resp client.FolderListResponse
		if err := c.Do("GET", "/v1/folders", q, nil, &resp); err != nil {
			return err
		}
		for i := range resp.Folders {
			decryptFolderZK(dek, &resp.Folders[i])
		}

		if jsonOutput {
			return output.JSON(resp)
		}
		printFolderTable(resp.Folders)
		return nil
	},
}

func init() {
	folderListCmd.Flags().BoolVar(&folderListIncludeTrashed, "include-trashed", false, "include trashed folders")
	folderCmd.AddCommand(folderListCmd)
}

func printFolderTable(folders []client.Folder) {
	rows := make([][]string, 0, len(folders))
	for _, f := range folders {
		parent := "-"
		if f.ParentID != nil {
			parent = *f.ParentID
		}
		rows = append(rows, []string{f.ID, f.Name, parent, f.Status, f.UpdatedAt})
	}
	output.Table([]string{"ID", "NAME", "PARENT", "STATUS", "UPDATED"}, rows)
}
