package cmd

import (
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

var orgMembersCmd = &cobra.Command{
	Use:   "members",
	Short: "List your organization's members",
	Long: "List your organization's members. A plain member sees only their " +
		"own entry -- other members' emails/names/roles are admin/owner-only " +
		"information, scoped server-side the same way as the web app.",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var members []client.OrgMember
		if err := c.Do("GET", "/v1/organizations/me/members", nil, nil, &members); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(members)
		}
		printOrgMemberTable(members)
		return nil
	},
}

func init() {
	orgCmd.AddCommand(orgMembersCmd)
}

func printOrgMemberTable(members []client.OrgMember) {
	rows := make([][]string, 0, len(members))
	for _, m := range members {
		name := "-"
		if m.DisplayName != nil {
			name = *m.DisplayName
		}
		rows = append(rows, []string{m.Email, name, m.OrgRole, m.SubscriptionPlan})
	}
	output.Table([]string{"EMAIL", "NAME", "ROLE", "PLAN"}, rows)
}
