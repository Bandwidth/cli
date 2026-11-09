// Package build implements the `band build` command group: features of the Bandwidth Build API.
package build

import "github.com/spf13/cobra"

// Cmd is the `band build` parent command.
var Cmd = &cobra.Command{
	Use:   "build",
	Short: "Manage Bandwidth Build account features",
	Long: "Manage features of a Bandwidth Build account through the Build API. " +
		"SIP realms and credentials for digest authentication live under 'band sip'.",
	Example: `  band build sip-trunk --transport TLS --plain`,
}
