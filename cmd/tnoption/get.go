package tnoption

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
	Use:     "get <orderId>",
	Short:   "Get the status of a TN Option Order",
	Example: `  band tnoption get ddbdc72e-dc27-490c-904e-d0c11291b095`,
	Args:    cobra.ExactArgs(1),
	RunE:    runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(fmt.Sprintf("/accounts/%s/tnoptions/%s", acctID, args[0]), &result); err != nil {
		return fmt.Errorf("getting TN option order: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
