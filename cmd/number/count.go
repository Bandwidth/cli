package number

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	countSubaccount   string
	countLocation     string
	countDisconnected bool
)

func init() {
	countCmd.Flags().StringVar(&countSubaccount, "subaccount", "", "Count numbers on a sub-account (site ID)")
	countCmd.Flags().StringVar(&countLocation, "location", "", "Count numbers on a location (SIP peer ID); requires --subaccount")
	countCmd.Flags().BoolVar(&countDisconnected, "disconnected", false, "Count disconnected numbers instead of in-service ones")
	Cmd.AddCommand(countCmd)
}

var countCmd = &cobra.Command{
	Use:   "count",
	Short: "Count phone numbers without listing them",
	Long: `Returns the number of phone numbers on the account, a sub-account, or a
location using the Dashboard totals endpoints — no paging through the
full inventory.`,
	Example: `  band number count
  band number count --disconnected
  band number count --subaccount 407
  band number count --subaccount 407 --location 500017`,
	RunE: runCount,
}

// countPath maps count flags to the matching totals endpoint.
func countPath(acctID string, subaccount, location string, disconnected bool) (string, error) {
	if disconnected && (subaccount != "" || location != "") {
		return "", cmdutil.NewFlagError("--disconnected cannot be combined with --subaccount or --location")
	}
	if location != "" && subaccount == "" {
		return "", cmdutil.NewFlagError("--location requires --subaccount")
	}
	switch {
	case disconnected:
		return fmt.Sprintf("/accounts/%s/discnumbers/totals", acctID), nil
	case subaccount != "" && location != "":
		return fmt.Sprintf("/accounts/%s/sites/%s/sippeers/%s/totaltns", acctID, subaccount, location), nil
	case subaccount != "":
		return fmt.Sprintf("/accounts/%s/sites/%s/totaltns", acctID, subaccount), nil
	default:
		return fmt.Sprintf("/accounts/%s/inserviceNumbers/totals", acctID), nil
	}
}

func runCount(cmd *cobra.Command, args []string) error {
	// Validate flags before authenticating so misuse fails fast.
	if _, err := countPath("x", countSubaccount, countLocation, countDisconnected); err != nil {
		return err
	}

	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	path, err := countPath(acctID, countSubaccount, countLocation, countDisconnected)
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return fmt.Errorf("counting phone numbers: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, output.FlattenAndNormalize(result))
}
