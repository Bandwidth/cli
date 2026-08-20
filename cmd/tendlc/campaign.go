package tendlc

import (
	"github.com/spf13/cobra"
)

// campaignCmd is the `band tendlc campaign` parent.
var campaignCmd = &cobra.Command{
	Use:   "campaign",
	Short: "Manage 10DLC campaigns",
	Long: `Register and manage 10DLC campaigns.

A campaign belongs to exactly one brand, and that brand must reach VERIFIED
or VETTED_VERIFIED before the campaign can carry traffic.

Requires the Registration Center feature and the Campaign Management role.
Check with 'band tendlc status --plain'.`,
}

func init() {
	Cmd.AddCommand(campaignCmd)
}
