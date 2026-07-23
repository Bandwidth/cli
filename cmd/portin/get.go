package portin

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
	Use:     "get <order-id>",
	Short:   "Get the current status of a port-in order",
	Example: `  band portin get b9ef682b-2b42-4287-bfe4-ba03ec57cb07`,
	Args:    cobra.ExactArgs(1),
	RunE:    runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(fmt.Sprintf("/accounts/%s/portins/%s", acctID, args[0]), &result); err != nil {
		return portinError(err, "getting port-in order")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, flattenPortInResult(result, args[0]))
	}
	return output.StdoutAuto(format, plain, result)
}
