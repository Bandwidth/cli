// Package bulk implements the `band portin bulk` command surface for managing
// bulk port-in orders. A bulk order accepts a large list of TNs, runs an
// asynchronous portability validation, and decomposes into one or more child
// port-in orders that can then be driven through the standard `band portin`
// lifecycle.
package bulk

import "github.com/spf13/cobra"

// Cmd is the `band portin bulk` parent command.
var Cmd = &cobra.Command{
	Use:   "bulk",
	Short: "Manage bulk port-in orders",
	Long: `Bulk port-ins accept a large TN list and split it into validated child
port-in orders. The TN list validation is asynchronous — submit with
` + "`bulk create`" + `, then poll completion with ` + "`bulk get-tns --wait`" + `.
Child orders are managed through the standard ` + "`band portin <subcommand>`" + `.`,
}
