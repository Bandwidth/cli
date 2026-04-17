package number

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	orderWait    bool
	orderTimeout time.Duration
)

func init() {
	orderCmd.Flags().BoolVar(&orderWait, "wait", false, "Wait until the ordered number(s) appear in service")
	orderCmd.Flags().DurationVar(&orderTimeout, "timeout", 30*time.Second, "Maximum time to wait (default 30s)")
	Cmd.AddCommand(orderCmd)
}

var orderCmd = &cobra.Command{
	Use:   "order [number...]",
	Short: "Order one or more phone numbers",
	Long:  "Orders one or more phone numbers from a search result. Use --wait to block until the numbers are active.",
	Example: `  band number order +19195551234
  band number order +19195551234 +19195551235
  band number order +19195551234 --wait --timeout 30s`,
	Args: cobra.MinimumNArgs(1),
	RunE: runOrder,
}

// BuildOrderBody builds the XML request body for ordering phone numbers.
func BuildOrderBody(numbers []string) map[string]interface{} {
	return map[string]interface{}{
		"TelephoneNumberList": map[string]interface{}{
			"TelephoneNumber": numbers,
		},
	}
}

func runOrder(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	bodyData := BuildOrderBody(args)

	var result interface{}
	if err := client.Post(fmt.Sprintf("/accounts/%s/orders", acctID), api.XMLBody{RootElement: "Order", Data: bodyData}, &result); err != nil {
		return fmt.Errorf("ordering numbers: %w", err)
	}

	if !orderWait {
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, result)
	}

	// Poll until all ordered numbers appear as Inservice. InAccount means the
	// number is assigned but not yet routable, so we wait for Inservice only —
	// otherwise --wait would return too early.
	ordered := make(map[string]bool, len(args))
	for _, n := range args {
		ordered[normalizeE164(n)] = true
	}

	final, err := cmdutil.Poll(cmdutil.PollConfig{
		Interval: 2 * time.Second,
		Timeout:  orderTimeout,
		Check: func() (bool, interface{}, error) {
			nums, err := fetchAccountNumbers(client, acctID, "Inservice")
			if err != nil {
				return false, nil, fmt.Errorf("polling in-service numbers: %w", err)
			}
			found := 0
			for _, n := range nums {
				if ordered[n] {
					found++
				}
			}
			if found >= len(args) {
				return true, nums, nil
			}
			return false, nil, nil
		},
	})
	if err != nil {
		return err
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, final)
}
