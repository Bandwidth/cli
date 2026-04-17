package location

import "github.com/spf13/cobra"

// Cmd is the `band location` parent command.
var Cmd = &cobra.Command{
	Use:   "location",
	Short: "Manage locations (SIP peers) under sub-accounts",
}
