package vcp

import "github.com/spf13/cobra"

// Cmd is the `band vcp` parent command.
var Cmd = &cobra.Command{
	Use:   "vcp",
	Short: "Manage Voice Configuration Packages (Universal Platform)",
}
