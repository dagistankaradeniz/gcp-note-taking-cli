package cmd

import (
	"github.com/spf13/cobra"
)

// Read-only, deliberately: no `org invite`/`org remove`/`org set-role`/etc
// -- v1 has no organizations:write scope yet (see backend's
// app/routers/v1_organizations.py docstring). Those admin actions stay
// web-only for now.
var orgCmd = &cobra.Command{
	Use:   "org",
	Short: "View your organization (Team plan)",
}

func init() {
	rootCmd.AddCommand(orgCmd)
}
