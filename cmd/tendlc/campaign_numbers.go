package tendlc

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	campaignNumbersLimit  int
	campaignNumbersOffset int
)

func init() {
	campaignNumbersCmd.Flags().IntVar(&campaignNumbersLimit, "limit", 50, "Page size (max 250)")
	campaignNumbersCmd.Flags().IntVar(&campaignNumbersOffset, "offset", 0, "Pagination offset")
	campaignsCmd.AddCommand(campaignNumbersCmd)
}

var campaignNumbersCmd = &cobra.Command{
	Use:   "numbers <campaign-id>",
	Short: "List phone numbers assigned to a campaign",
	Long:  "Shows all phone numbers associated with a specific 10DLC campaign, including numbers with provisioning errors.",
	Example: `  band tendlc campaigns numbers CR8HFN0`,
	Args:  cobra.ExactArgs(1),
	RunE:  runCampaignNumbers,
}

func runCampaignNumbers(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v2/accounts/%s/tendlc/campaigns/%s/phoneNumbers?limit=%d&offset=%d",
		acctID, url.PathEscape(args[0]), campaignNumbersLimit, campaignNumbersOffset)

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return roleGateError(err, "Campaign Management")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, extractData(result))
}
