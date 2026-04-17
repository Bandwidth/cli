package tendlc

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	numbersLimit      int
	numbersOffset     int
	numbersCampaignID string
	numbersStatus     string
)

func init() {
	numbersCmd.Flags().IntVar(&numbersLimit, "limit", 50, "Page size (max 250)")
	numbersCmd.Flags().IntVar(&numbersOffset, "offset", 0, "Pagination offset")
	numbersCmd.Flags().StringVar(&numbersCampaignID, "campaign-id", "", "Filter by campaign ID")
	numbersCmd.Flags().StringVar(&numbersStatus, "status", "", "Filter by status: PROCESSING, SUCCESS, FAILURE")
	Cmd.AddCommand(numbersCmd)
	Cmd.AddCommand(numberGetCmd)
}

var numbersCmd = &cobra.Command{
	Use:   "numbers",
	Short: "List 10DLC registered phone numbers",
	Long:  "Lists all phone numbers registered for A2P 10DLC traffic, with their campaign assignment and registration status.",
	Example: `  # List all registered numbers
  band tendlc numbers

  # Filter by campaign
  band tendlc numbers --campaign-id CR8HFN0

  # Filter by status
  band tendlc numbers --status SUCCESS`,
	RunE: runNumbers,
}

func runNumbers(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v2/accounts/%s/tendlc/phoneNumbers?limit=%d&offset=%d",
		acctID, numbersLimit, numbersOffset)

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return roleGateError(err, "Campaign Management")
	}

	data := extractData(result)

	// Client-side filtering — the phoneNumbers endpoint doesn't support
	// server-side filtering on status or campaignId.
	if numbersStatus != "" || numbersCampaignID != "" {
		data = filterNumbers(data, numbersStatus, numbersCampaignID)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, data)
}

var numberGetCmd = &cobra.Command{
	Use:   "number <phoneNumber>",
	Short: "Get 10DLC registration details for a phone number",
	Long:  "Shows the 10DLC registration status, campaign assignment, and brand for a specific phone number.",
	Example: `  band tendlc number +19195551234`,
	Args:  cobra.ExactArgs(1),
	RunE:  runNumberGet,
}

func runNumberGet(cmd *cobra.Command, args []string) error {
	number := cmdutil.NormalizeNumber(args[0])

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v2/accounts/%s/tendlc/phoneNumbers/%s", acctID, url.PathEscape(number))

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return roleGateError(err, "Campaign Management")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
