package portin

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	submitWait    bool
	submitTimeout time.Duration
)

func init() {
	submitCmd.Flags().BoolVar(&submitWait, "wait", false, "Wait until the order leaves VALIDATE_TFNS")
	submitCmd.Flags().DurationVar(&submitTimeout, "timeout", 120*time.Second, "Maximum time to wait (default 120s)")
	Cmd.AddCommand(submitCmd)
}

var submitCmd = &cobra.Command{
	Use:     "submit <order-id>",
	Short:   "Submit a draft port-in order to Neustar / SOMOS",
	Long:    `Transitions a draft port-in order into the SUBMITTED state, sending it on to the porting vendor. With --wait, blocks until the order leaves VALIDATE_TFNS and reaches PENDING_DOCUMENTS / FOC_GRANTED / a failed state.`,
	Example: `  band portin submit b9ef682b-2b42-4287-bfe4-ba03ec57cb07 --wait`,
	Args:    cobra.ExactArgs(1),
	RunE:    runSubmit,
}

func runSubmit(cmd *cobra.Command, args []string) error {
	orderID := args[0]
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	body := map[string]interface{}{
		"ProcessingStatus": "SUBMITTED",
	}

	var result interface{}
	if err := client.Put(
		fmt.Sprintf("/accounts/%s/portins/%s", acctID, orderID),
		api.XMLBody{RootElement: "LnpOrderSupp", Data: body},
		&result,
	); err != nil {
		return portinError(err, "submitting port-in order")
	}

	if submitWait {
		final, err := waitForSubmitted(client, acctID, orderID, submitTimeout)
		if err != nil {
			return err
		}
		result = final
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, flattenPortInResult(result, orderID))
	}
	return output.StdoutAuto(format, plain, result)
}

// waitForSubmitted polls the order until it leaves VALIDATE_TFNS / SUBMITTED
// and reaches a state where the user has actionable next steps.
func waitForSubmitted(client *api.Client, acctID, orderID string, timeout time.Duration) (interface{}, error) {
	terminal := map[string]bool{
		"PENDING_DOCUMENTS": true,
		"FOC_GRANTED":       true,
		"FOC":               true,
		"COMPLETE":          true,
		"REJECTED":          true,
		"FAILED":            true,
		"INVALID_DRAFT_TFNS": true,
	}
	return cmdutil.Poll(cmdutil.PollConfig{
		Interval: 3 * time.Second,
		Timeout:  timeout,
		Check: func() (bool, interface{}, error) {
			var r interface{}
			if err := client.Get(
				fmt.Sprintf("/accounts/%s/portins/%s", acctID, orderID),
				&r,
			); err != nil {
				return false, nil, portinError(err, "polling order")
			}
			status := strings.ToUpper(digString(r, "ProcessingStatus"))
			if terminal[status] {
				return true, r, nil
			}
			return false, nil, nil
		},
	})
}
