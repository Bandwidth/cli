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
	Args: cobra.NoArgs,
	// A trivial RunE is required, not decorative: cobra's execute() checks
	// Runnable() (Run/RunE set) BEFORE it ever calls ValidateArgs, so a
	// parent with no RunE always short-circuits to flag.ErrHelp regardless
	// of Args -- Args: cobra.NoArgs above would silently never run. Calling
	// cmd.Help() here keeps the existing bare-invocation behavior (print
	// help, exit 0) while letting a stray positional -- e.g. a deleted
	// command's name -- reach NoArgs and fail with a non-zero exit instead
	// of being silently swallowed as if it were a help request.
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(campaignCmd)
}
