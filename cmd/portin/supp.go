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
	suppTimeout time.Duration
)

func init() {
	suppCmd.Flags().StringVar(&suppFOCDate, "foc", "", "Requested FOC date (ISO 8601)")
	suppCmd.Flags().StringVar(&suppSiteID, "site", "", "Site (sub-account) ID to switch the order to")
	suppCmd.Flags().StringVar(&suppPeerID, "peer", "", "SIP peer (location) ID to switch the order to")
	suppCmd.Flags().DurationVar(&suppTimeout, "timeout", 30*time.Second, "Maximum time to wait for propagation (default 30s)")
	Cmd.AddCommand(suppCmd)
}

var suppCmd = &cobra.Command{
	Use:   "supp <order-id>",
	Short: "Supplement an existing port-in order (change FOC, site, peer, etc.)",
	Long: `Sends a supplement (PUT) to an existing port-in order and waits for the
change to propagate. The Bandwidth API has a documented behavior where a
supp on a wireless_to_wireless order past FOC returns 200 on the PUT but
sets error code 7300 on the next GET — meaning Neustar never received the
change. This command always polls until either the order's last-modified
timestamp advances past the pre-PUT value, or 7300 surfaces, or the
timeout expires. Exit 1 on 7300 with a clear message; exit 5 on timeout.`,
	Example: `  band portin supp b9ef682b-2b42-4287-bfe4-ba03ec57cb07 --foc 2026-06-01Z
  band portin supp b9ef682b --site 1234 --peer 5678 --timeout 60s`,
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

	// Capture the pre-PUT lastModifiedDate so we can detect actual propagation
	// rather than guessing.
	var pre interface{}
	if err := client.Get(fmt.Sprintf("/accounts/%s/portins/%s", acctID, orderID), &pre); err != nil {
		return portinError(err, "fetching order before supplement")
	}
	preTS := digString(pre, "LastModifiedDate")

	var putResult interface{}
	if err := client.Put(
		fmt.Sprintf("/accounts/%s/portins/%s", acctID, orderID),
		api.XMLBody{RootElement: "LnpOrderSupp", Data: body},
		&putResult,
	); err != nil {
		return portinError(err, "supplementing port-in order")
	}

	verified, err := waitForSuppPropagation(client, acctID, orderID, preTS, suppTimeout)
	if err != nil {
		return err
	}
	if is7300(verified) {
		return fmt.Errorf("supplement was accepted by the API but did not propagate to Neustar — typically because the order is in a state where supps are blocked (e.g., wireless_to_wireless after FOC, or post-FOC field changes). Your change has not taken effect")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, flattenPortInResult(verified, orderID))
	}
	return output.StdoutAuto(format, plain, verified)
}

// waitForSuppPropagation polls until the order's LastModifiedDate advances
// past the pre-PUT timestamp (real propagation), or error code 7300 appears
// (silent failure), or the timeout expires.
func waitForSuppPropagation(client *api.Client, acctID, orderID, preTS string, timeout time.Duration) (interface{}, error) {
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
			if is7300(r) {
				return true, r, nil
			}
			cur := digString(r, "LastModifiedDate")
			if cur != "" && cur != preTS {
				return true, r, nil
			}
			return false, nil, nil
		},
	})
}
