package bulk

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:     "get <bulk-order-id>",
	Short:   "Get the current state of a bulk port-in order",
	Example: `  band portin bulk get b3d89f9e-a46e-4d56-aaad-9c9d8ac98bb9`,
	Args:    cobra.ExactArgs(1),
	RunE:    runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(fmt.Sprintf("/accounts/%s/bulkPortins/%s", acctID, args[0]), &result); err != nil {
		return bulkError(err, "getting bulk port-in")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, flattenBulkResult(result))
	}
	return output.StdoutAuto(format, plain, result)
}
