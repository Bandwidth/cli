package number

import (
	"github.com/spf13/cobra"
)

func init() {
	Cmd.AddCommand(deactivateCmd)
	registerServiceActivationFlags(deactivateCmd)
}

var deactivateCmd = &cobra.Command{
	Use:   "deactivate <number...>",
	Short: "Deactivate voice or messaging services on phone numbers",
	Long: `Creates a service deactivation order to disable voice and/or messaging
services on one or more phone numbers via the Universal Platform.

At least one service flag must be provided. Use --dry-run to inspect
the eligibility matrix (which mirrors activate's). Use --wait to block
until the order reaches a terminal status.

Underlying API: POST /api/v2/accounts/{accountId}/serviceActivation
with action=DEACTIVATE`,
	Example: `  # Disable inbound voice on a number
  band number deactivate +19195551234 --voice-inbound

  # Disable inbound voice and wait for the order to settle
  band number deactivate +19195551234 --voice-inbound --wait`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServiceActivation(cmd, "DEACTIVATE", args)
	},
}
