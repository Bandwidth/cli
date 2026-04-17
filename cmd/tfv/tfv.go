package tfv

import "github.com/spf13/cobra"

// Cmd is the `band tfv` parent command.
var Cmd = &cobra.Command{
	Use:   "tfv",
	Short: "Toll-free verification management",
	Long: `Check and manage toll-free number verification status.

Requires the TFV role on your account. If you get a 403 error,
contact your Bandwidth account manager to enable the role.`,
}
