package app

import "github.com/spf13/cobra"

// Cmd is the `band app` parent command.
var Cmd = &cobra.Command{
	Use:   "app",
	Short: "Manage Bandwidth applications",
}
