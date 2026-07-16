package vcp

import "github.com/spf13/cobra"

// Cmd is the `band vcp` parent command.
var Cmd = &cobra.Command{
	Use:   "vcp",
	Short: "Manage Voice Configuration Packages (Universal Platform)",
	Long:  "Create, inspect, and manage Voice Configuration Packages (VCPs) on the Universal Platform. VCPs define voice routing and settings for groups of phone numbers, and can be linked to a voice application for HTTP callbacks.",
}
