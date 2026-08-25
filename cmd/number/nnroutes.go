package number

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(nnroutesCmd)
}

var nnroutesCmd = &cobra.Command{
	Use:   "nnroutes <number>",
	Short: "List the NetNumber routes available to a phone number",
	Long: `Lists the NetNumber (NN) routes available to a phone number, each with
its NNID and name. The route currently assigned to the number is shown by
"band number details" under MessagingSettings.`,
	Example: `  band number nnroutes +19195551234`,
	Args:    cobra.ExactArgs(1),
	RunE:    runNNRoutes,
}

func runNNRoutes(cmd *cobra.Command, args []string) error {
	number := cmdutil.NormalizeE164(args[0])

	client, _, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(cmd.Context(), fmt.Sprintf("/tns/%s/availableNnRoutes", number), &result); err != nil {
		return fmt.Errorf("listing NN routes: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, output.FlattenAndNormalize(result))
}
