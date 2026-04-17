package tnoption

import "github.com/spf13/cobra"

// Cmd is the `band tnoption` parent command.
var Cmd = &cobra.Command{
	Use:   "tnoption",
	Short: "Manage TN Option Orders (assign numbers to campaigns, set SMS/CNAM options)",
	Long: `Create and query TN Option Orders on the Bandwidth Dashboard API.

The most common use case is assigning phone numbers to 10DLC campaigns:

  band tnoption assign +19195551234 --campaign-id CA3XKE1

TN Option Orders can also enable/disable SMS, set CNAM display, configure
port-out passcodes, and more. Use "band tnoption create" for full control.`,
}
