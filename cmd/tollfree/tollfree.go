// Package tollfree implements `band tollfree`, read commands for toll-free
// routing configuration.
package tollfree

import "github.com/spf13/cobra"

// Cmd is the `band tollfree` parent command.
var Cmd = &cobra.Command{
	Use:   "tollfree",
	Short: "Toll-free routing reads",
	Long: `Read toll-free routing configuration for numbers on the account.

Toll-free routing template search is gated per account and is off by
default. If you get a 403 error, ask your Bandwidth account manager to
enable toll-free template search on the account.`,
}
