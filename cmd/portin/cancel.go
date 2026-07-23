package portin

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(cancelCmd)
}

var cancelCmd = &cobra.Command{
	Use:     "cancel <order-id>",
	Short:   "Cancel a port-in order",
	Long:    `Cancels a port-in order. Cancellation is typically irreversible — the order cannot be reactivated.`,
	Example: `  band portin cancel b9ef682b-2b42-4287-bfe4-ba03ec57cb07`,
	Args:    cobra.ExactArgs(1),
	RunE:    runCancel,
}

func runCancel(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Delete(fmt.Sprintf("/accounts/%s/portins/%s", acctID, args[0]), &result); err != nil {
		return portinError(err, "cancelling port-in order")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, map[string]interface{}{
			"orderId": args[0],
			"status":  "CANCELLED",
		})
	}
	return output.StdoutAuto(format, plain, result)
}
