package recording

import "github.com/spf13/cobra"

// Cmd is the `band recording` parent command.
var Cmd = &cobra.Command{
	Use:   "recording",
	Short: "Manage call recordings",
}
