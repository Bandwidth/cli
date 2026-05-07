package bulk

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	getTnsWait    bool
	getTnsTimeout time.Duration
)

func init() {
	getTnsCmd.Flags().BoolVar(&getTnsWait, "wait", false, "Wait until validation reaches VALID_DRAFT_TNS or INVALID_DRAFT_TNS")
	getTnsCmd.Flags().DurationVar(&getTnsTimeout, "timeout", 120*time.Second, "Maximum time to wait (default 120s)")
	Cmd.AddCommand(getTnsCmd)
}

var getTnsCmd = &cobra.Command{
	Use:     "get-tns <bulk-order-id>",
	Short:   "Poll the TN-list validation for a bulk port-in order",
	Long:    `Polls the asynchronous TN-list validation for a bulk port-in. With --wait, blocks until the validation completes (VALID_DRAFT_TNS or INVALID_DRAFT_TNS).`,
	Example: `  band portin bulk get-tns b3d89f9e-a46e-4d56-aaad-9c9d8ac98bb9 --wait`,
	Args:    cobra.ExactArgs(1),
	RunE:    runGetTns,
}

func runGetTns(cmd *cobra.Command, args []string) error {
	orderID := args[0]

	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	get := func() (interface{}, error) {
		var r interface{}
		if err := client.Get(
			fmt.Sprintf("/accounts/%s/bulkPortins/%s/tnList", acctID, orderID),
			&r,
		); err != nil {
			return nil, bulkError(err, "polling bulk TN list")
		}
		return r, nil
	}

	var result interface{}
	if !getTnsWait {
		result, err = get()
		if err != nil {
			return err
		}
	} else {
		final, perr := cmdutil.Poll(cmdutil.PollConfig{
			Interval: 3 * time.Second,
			Timeout:  getTnsTimeout,
			Check: func() (bool, interface{}, error) {
				r, err := get()
				if err != nil {
					return false, nil, err
				}
				switch strings.ToUpper(digString(r, "ProcessingStatus")) {
				case "VALID_DRAFT_TNS", "INVALID_DRAFT_TNS":
					return true, r, nil
				default:
					return false, nil, nil
				}
			},
		})
		if perr != nil {
			return perr
		}
		result = final
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, flattenBulkResult(result))
	}
	return output.StdoutAuto(format, plain, result)
}
