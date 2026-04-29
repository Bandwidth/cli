package number

import (
	"github.com/spf13/cobra"
)

func init() {
	Cmd.AddCommand(activateCmd)
	registerServiceActivationFlags(activateCmd)
}

var activateCmd = &cobra.Command{
	Use:   "activate <number...>",
	Short: "Activate voice or messaging services on phone numbers",
	Long: `Creates a service activation order to enable voice and/or messaging
services on one or more phone numbers via the Universal Platform.

At least one service flag must be provided. Use --dry-run to check
eligibility (status per service) without creating an order. Use --wait
to block until the order reaches a terminal status.

Underlying API: POST /api/v2/accounts/{accountId}/serviceActivation`,
	Example: `  # Enable inbound voice on a single number
  band number activate +19195551234 --voice-inbound

  # Enable all voice services on multiple numbers and wait
  band number activate +19195551234 +19195551235 --voice-inbound \
    --voice-outbound-national --voice-outbound-international --wait

  # Eligibility check only — no order created
  band number activate +19195551234 --voice-inbound --dry-run

  # With a customer-supplied order ID for tracking
  band number activate +19195551234 --voice-inbound --customer-order-id my-order-123`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServiceActivation(cmd, "ACTIVATE", args)
	},
}
