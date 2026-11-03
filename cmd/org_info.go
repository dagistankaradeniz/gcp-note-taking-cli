package cmd

import (
	"fmt"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
	"github.com/spf13/cobra"
)

var orgInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show your organization's plan, seats, and status",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var org client.Organization
		if err := c.Do("GET", "/v1/organizations/me", nil, nil, &org); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(org)
		}
		if richTable() {
			fields := []ui.Field{
				{Label: "Plan", Value: planName(org.PlanTier)},
				{Label: "Members", Value: fmt.Sprint(org.MemberCount)},
				{Label: "Seats", Value: fmt.Sprintf("%d total · free %d · plus %d · pro %d", org.SeatCount, org.SeatAllocations.Free, org.SeatAllocations.Plus, org.SeatAllocations.Pro)},
			}
			if org.Frozen {
				fields = append(fields, ui.Field{Label: "Status", Value: ui.Fg(ui.Red, "frozen", true)})
			}
			if org.SsoStatus != "not_configured" {
				fields = append(fields, ui.Field{Label: "SSO", Value: org.SsoStatus})
			}
			for _, alert := range org.Alerts {
				fields = append(fields, ui.Field{Label: "Alert", Value: ui.Fg(ui.Amber, fmt.Sprintf("%s (%s, %d days remaining)", alert.Type, alert.Severity, alert.DaysRemaining), false)})
			}
			fmt.Print(ui.Card(org.Name, fields))
			return nil
		}
		fmt.Printf("%s (%s)\n", org.Name, org.PlanTier)
		fmt.Printf("Members: %d\n", org.MemberCount)
		fmt.Printf("Seats:   free=%d plus=%d pro=%d (of %d total)\n",
			org.SeatAllocations.Free, org.SeatAllocations.Plus, org.SeatAllocations.Pro, org.SeatCount)
		if org.Frozen {
			fmt.Println("Status:  frozen")
		}
		if org.SsoStatus != "not_configured" {
			fmt.Printf("SSO:     %s\n", org.SsoStatus)
		}
		for _, alert := range org.Alerts {
			fmt.Printf("Alert:   %s (%s, %d days remaining)\n", alert.Type, alert.Severity, alert.DaysRemaining)
		}
		return nil
	},
}

func init() {
	orgCmd.AddCommand(orgInfoCmd)
}
