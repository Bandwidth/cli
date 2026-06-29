package number

import "github.com/spf13/cobra"

// Cmd is the `band number` parent command.
var Cmd = &cobra.Command{
	Use:   "number",
	Short: "Manage Bandwidth phone numbers",
	Long:  "Search, order, activate, inspect, and release phone numbers. Numbers are referenced in E.164 format (e.g. +19195551234) throughout these commands.",
}
