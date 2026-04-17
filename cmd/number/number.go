package number

import "github.com/spf13/cobra"

// Cmd is the `band number` parent command.
var Cmd = &cobra.Command{
	Use:   "number",
	Short: "Manage Bandwidth phone numbers",
}
