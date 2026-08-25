package portin

import (
	"context"
	"errors"
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
	Long:    `Transitions a draft port-in order into the SUBMITTED state, sending it on to the porting vendor. With --wait, blocks until the order leaves VALIDATE_TFNS — typically SUBMITTED (validation passed, order is with the vendor) or INVALID_TFNS (one or more TNs not portable).`,
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
		cmd.Context(),
		fmt.Sprintf("/accounts/%s/portins/%s", acctID, orderID),
		api.XMLBody{RootElement: "LnpOrderSupp", Data: body},
		&result,
	); err != nil {
		return portinError(err, "submitting port-in order")
	}

	if submitWait {
		final, err := waitForSubmitted(cmd.Context(), client, acctID, orderID, submitTimeout)
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

// submitTerminal is the set of post-submit states where --wait stops
// polling. SUBMITTED is the happy terminal for toll-free orders: it means
// TFN validation (VALIDATE_TFNS) passed and the order is with the vendor.
// INVALID_TFNS is the post-submit validation failure. Draft-side states
// (DRAFT, VALID_DRAFT_TFNS) are deliberately absent: right after the PUT
// the order may briefly report them before transitioning, and returning
// then would defeat the wait. INVALID_DRAFT_TFNS is kept as a defensive
// catch for orders bounced back to draft. REJECTED, FAILED, and FOC_GRANTED
// are not in the API spec but are kept as a defensive net.
var submitTerminal = map[string]bool{
	"SUBMITTED":                true,
	"INVALID_TFNS":             true,
	"INVALID_DRAFT_TFNS":       true,
	"EXCEPTION":                true,
	"PENDING_DOCUMENTS":        true,
	"PENDING_CARRIER_APPROVAL": true,
	"REQUESTED_SUPP":           true,
	"FOC":                      true,
	"COMPLETE":                 true,
	"CANCELLED":                true,
	"REQUESTED_CANCEL":         true,
	"REJECTED":                 true,
	"FAILED":                   true,
	"FOC_GRANTED":              true,
}

// submitWaitDone reports whether waitForSubmitted should stop polling for
// the given current and previously observed statuses.
func submitWaitDone(status, prevStatus string) bool {
	if status == "SUBMITTED" {
		// Right after the submit PUT the order can transiently report
		// SUBMITTED before entering VALIDATE_TFNS; a single sighting is not
		// proof validation passed.
		return prevStatus == "SUBMITTED" || prevStatus == "VALIDATE_TFNS"
	}
	return submitTerminal[status]
}

// waitForSubmitted polls the order until it leaves the transient
// VALIDATE_TFNS state and reaches a state where the user has actionable
// next steps. On timeout, the submit itself has already been accepted —
// the error says so and reports the last-seen status instead of leaving
// the user guessing whether the command worked.
func waitForSubmitted(ctx context.Context, client *api.Client, acctID, orderID string, timeout time.Duration) (interface{}, error) {
	lastStatus := ""
	prevStatus := ""
	result, err := cmdutil.Poll(cmdutil.PollConfig{
		Context:  ctx,
		Interval: 3 * time.Second,
		Timeout:  timeout,
		Check: func() (bool, interface{}, error) {
			var r interface{}
			if err := client.Get(
				ctx,
				fmt.Sprintf("/accounts/%s/portins/%s", acctID, orderID),
				&r,
			); err != nil {
				return false, nil, portinError(err, "polling order")
			}
			status := strings.ToUpper(digString(r, "ProcessingStatus"))
			done := submitWaitDone(status, prevStatus)
			prevStatus = status
			lastStatus = status
			if done {
				return true, r, nil
			}
			return false, nil, nil
		},
	})
	if errors.Is(err, cmdutil.ErrPollTimeout) {
		if lastStatus == "VALIDATE_TFNS" {
			return nil, fmt.Errorf(
				"the submit was accepted, but the order is still in %s after %s — toll-free validation time grows with the number of TNs. Check progress with: band portin get %s (%w)",
				lastStatus, timeout, orderID, cmdutil.ErrPollTimeout)
		}
		return nil, fmt.Errorf(
			"the submit was accepted, but the order is still in %s after %s. Check progress with: band portin get %s (%w)",
			lastStatus, timeout, orderID, cmdutil.ErrPollTimeout)
	}
	return result, err
}
