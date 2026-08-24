package tendlc

import (
	"github.com/spf13/cobra"
)

// brandCmd is the `band tendlc brand` parent.
var brandCmd = &cobra.Command{
	Use:   "brand",
	Short: "Manage 10DLC brands",
	Long: `Register and manage 10DLC brands.

A brand needs a customer profile first, and a profile backs exactly one brand:
'band customer-profile create' then 'band tendlc brand create'. A brand must
reach VERIFIED or VETTED_VERIFIED before it can carry campaigns.

Brands have two IDs. bandwidthId exists immediately; brandId is assigned by TCR
and is null until registration completes. Commands here accept either.

Requires the Registration Center feature and the Campaign Management role.
Check with 'band tendlc status --plain'.`,
}

func init() {
	Cmd.AddCommand(brandCmd)
}
