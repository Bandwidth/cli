package recording

import "github.com/spf13/cobra"

// Cmd is the `band recording` parent command.
var Cmd = &cobra.Command{
	Use:   "recording",
	Short: "Manage call recordings",
	Long:  "List, inspect, download, and delete recordings produced by voice calls. Recordings are tied to the call that created them, so most commands take a callId.",
}
