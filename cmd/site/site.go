package site

import "github.com/spf13/cobra"

// Cmd is the `band site` parent command.
var Cmd = &cobra.Command{
	Use:     "subaccount",
	Aliases: []string{"site"},
	Short:   "Manage sub-accounts",
	Long:    "Sub-accounts (formerly called sites) are the top-level organizational unit in Bandwidth's legacy account hierarchy. For Universal Platform accounts, use 'band vcp' instead.",
}
