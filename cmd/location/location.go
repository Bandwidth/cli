package location

import "github.com/spf13/cobra"

// Cmd is the `band location` parent command.
var Cmd = &cobra.Command{
	Use:   "location",
	Short: "Manage locations (SIP peers) under sub-accounts",
	Long:  "Create and list locations (SIP peers) within a sub-account. Locations define the SIP endpoints that route voice traffic for the numbers assigned to them.",
}
