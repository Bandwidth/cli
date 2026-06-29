package app

import "github.com/spf13/cobra"

// Cmd is the `band app` parent command.
var Cmd = &cobra.Command{
	Use:   "app",
	Short: "Manage Bandwidth applications",
	Long:  "Create, inspect, and manage Bandwidth applications. Applications hold the callback configuration that voice and messaging traffic is routed against, and are referenced by ID when sending messages or placing calls.",
}
