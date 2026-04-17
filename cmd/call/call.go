package call

import "github.com/spf13/cobra"

// Cmd is the `band call` parent command.
var Cmd = &cobra.Command{
	Use:   "call",
	Short: "Manage Bandwidth voice calls",
}
