// Package sample implements the `band sample` command group for downloading
// and running Bandwidth sample applications with auto-configured credentials.
package sample

import "github.com/spf13/cobra"

// Cmd is the `band sample` parent command.
var Cmd = &cobra.Command{
	Use:   "sample",
	Short: "Run Bandwidth sample applications",
	Long: `Download and run Bandwidth sample applications with credentials automatically
wired from your active band profile.

Examples:
  band sample list
  band sample run live-assistant --language python
band sample run live-assistant --language python --call-to +19195550101`,
}

func init() {
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(runCmd)
}
