package number

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	"github.com/Bandwidth/cli/internal/ui"
)

var (
	orderWait       bool
	orderTimeout    time.Duration
	orderSubaccount string
)

func init() {
	orderCmd.Flags().StringVar(&orderSubaccount, "subaccount", "", "Sub-account ID to order the number(s) into (required; see `band subaccount list`)")
	orderCmd.Flags().BoolVar(&orderWait, "wait", false, "Wait until the ordered number(s) appear in service")
	orderCmd.Flags().DurationVar(&orderTimeout, "timeout", 30*time.Second, "Maximum time to wait (default 30s)")
	_ = orderCmd.MarkFlagRequired("subaccount")
	Cmd.AddCommand(orderCmd)
}

var orderCmd = &cobra.Command{
	Use:   "order [number...]",
	Short: "Order one or more phone numbers",
	Long:  "Orders one or more phone numbers (from a search result) into a sub-account. The Bandwidth orders API requires a sub-account, so --subaccount is required. Use --wait to block until the numbers are active.",
	Example: `  band number order +19195551234 --subaccount 152681
  band number order +19195551234 +19195551235 --subaccount 152681
  band number order +19195551234 --subaccount 152681 --wait --timeout 30s`,
	Args: cobra.MinimumNArgs(1),
	RunE: runOrder,
}

// BuildOrderBody builds the XML request body for ordering existing (available)
// phone numbers into a sub-account. The Bandwidth orders API requires the
// ExistingTelephoneNumberOrderType wrapper and a SiteId (sub-account); omitting
// either fails (bare TelephoneNumberList → HTTP 500; missing SiteId → 5022).
func BuildOrderBody(siteID string, numbers []string) map[string]interface{} {
	return map[string]interface{}{
		"SiteId": siteID,
		"ExistingTelephoneNumberOrderType": map[string]interface{}{
			"TelephoneNumberList": map[string]interface{}{
				"TelephoneNumber": numbers,
			},
		},
	}
}

func runOrder(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	bodyData := BuildOrderBody(orderSubaccount, args)

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
		// The order itself already succeeded; --wait only verifies that the
		// number reaches in-service. If this credential can't list numbers
		// (lacks the Numbers role → FeatureLimitError), don't report that as a
		// failure — surface the successful order and note we couldn't verify.
		var fle *cmdutil.FeatureLimitError
		if errors.As(err, &fle) {
			ui.Warnf("Order placed, but in-service status can't be verified with this credential (it lacks the Numbers role for listing). Check the Bandwidth dashboard, or re-run without --wait.")
			format, plain := cmdutil.OutputFlags(cmd)
			return output.StdoutAuto(format, plain, result)
		}
		return err
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, final)
}
