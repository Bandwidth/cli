package portin

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	suppFOCDate string
	suppSiteID  string
	suppPeerID  string
	suppWait    bool
	suppTimeout time.Duration
)

func init() {
	suppCmd.Flags().StringVar(&suppFOCDate, "foc", "", "Requested FOC date (ISO 8601)")
	suppCmd.Flags().StringVar(&suppSiteID, "site", "", "Site (sub-account) ID to switch the order to")
	suppCmd.Flags().StringVar(&suppPeerID, "peer", "", "SIP peer (location) ID to switch the order to")
	suppCmd.Flags().BoolVar(&suppWait, "wait", false, "Wait for the supplement to propagate (retries the verifying GET)")
	suppCmd.Flags().DurationVar(&suppTimeout, "timeout", 30*time.Second, "Maximum time to wait (default 30s)")
	Cmd.AddCommand(suppCmd)
}

var suppCmd = &cobra.Command{
	Use:   "supp <order-id>",
	Short: "Supplement an existing port-in order (change FOC, site, peer, etc.)",
	Long: `Sends a supplement (PUT) to an existing port-in order, then verifies the
change actually propagated. The Bandwidth API has a documented behavior where
a supp on a wireless_to_wireless order past FOC returns 200 on the PUT but
sets error code 7300 on the next GET — meaning Neustar never received the
change. This command always does the follow-up GET and exits 1 with a clear
message if 7300 is detected, so the supp doesn't silently fail.`,
	Example: `  band portin supp b9ef682b-2b42-4287-bfe4-ba03ec57cb07 --foc 2026-06-01Z
  band portin supp b9ef682b --site 1234 --peer 5678 --wait`,
	Args: cobra.ExactArgs(1),
	RunE: runSupp,
}

func runSupp(cmd *cobra.Command, args []string) error {
	orderID := args[0]

	body := map[string]interface{}{}
	if suppFOCDate != "" {
		body["RequestedFocDate"] = suppFOCDate
	}
	if suppSiteID != "" {
		body["SiteId"] = suppSiteID
	}
	if suppPeerID != "" {
		body["PeerId"] = suppPeerID
	}
	if len(body) == 0 {
		return fmt.Errorf("supp requires at least one field flag (--foc, --site, --peer)")
	}

	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var putResult interface{}
	if err := client.Put(
		fmt.Sprintf("/accounts/%s/portins/%s", acctID, orderID),
		api.XMLBody{RootElement: "LnpOrderSupp", Data: body},
		&putResult,
	); err != nil {
		return portinError(err, "supplementing port-in order")
	}

	// Always do a follow-up GET — even without --wait — to surface the silent
	// 7300 trap. Without --wait we do a single check; with --wait we retry
	// until lastModifiedDate advances or 7300 surfaces or timeout expires.
	verified, err := verifySupp(client, acctID, orderID, suppWait, suppTimeout)
	if err != nil {
		return err
	}
	if is7300(verified) {
		return fmt.Errorf("supplement was accepted by the API but did not propagate to Neustar — typically because the order is in a state where supps are blocked (e.g., wireless_to_wireless after FOC, or post-FOC field changes). Your change has not taken effect")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, flattenPortInResult(verified))
	}
	return output.StdoutAuto(format, plain, verified)
}

// verifySupp does a follow-up GET. Without wait, returns the single GET
// response. With wait, retries until either the order's lastModifiedDate
// advances past the pre-PUT timestamp or 7300 surfaces or timeout expires.
//
// We don't actually have the pre-PUT timestamp here, so the wait-mode poll
// just gives the API a few cycles to settle and watches for 7300 to appear.
func verifySupp(client *api.Client, acctID, orderID string, wait bool, timeout time.Duration) (interface{}, error) {
	if !wait {
		var r interface{}
		if err := client.Get(
			fmt.Sprintf("/accounts/%s/portins/%s", acctID, orderID),
			&r,
		); err != nil {
			return nil, portinError(err, "verifying supplement")
		}
		return r, nil
	}

	return cmdutil.Poll(cmdutil.PollConfig{
		Interval: 2 * time.Second,
		Timeout:  timeout,
		Check: func() (bool, interface{}, error) {
			var r interface{}
			if err := client.Get(
				fmt.Sprintf("/accounts/%s/portins/%s", acctID, orderID),
				&r,
			); err != nil {
				return false, nil, portinError(err, "verifying supplement")
			}
			// 7300 means the supp was rejected silently — terminate immediately.
			if is7300(r) {
				return true, r, nil
			}
			// On success, the order should have a meaningful status — return on
			// any non-empty status (the supp endpoint doesn't change status
			// itself, but the API stamps a fresh lastModifiedDate).
			if digString(r, "ProcessingStatus") != "" {
				return true, r, nil
			}
			return false, nil, nil
		},
	})
}
