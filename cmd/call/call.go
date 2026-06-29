package call

import "github.com/spf13/cobra"

// Cmd is the `band call` parent command.
var Cmd = &cobra.Command{
	Use:   "call",
	Short: "Manage Bandwidth voice calls",
	Long:  "Create, inspect, and control voice calls. Place outbound calls, redirect calls to new BXML, hang up active calls, and look up call state.",
}
