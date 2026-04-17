package tendlc

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	campaignsLimit  int
	campaignsOffset int
)

func init() {
	campaignsCmd.Flags().IntVar(&campaignsLimit, "limit", 50, "Page size (max 250)")
	campaignsCmd.Flags().IntVar(&campaignsOffset, "offset", 0, "Pagination offset")
	Cmd.AddCommand(campaignsCmd)
}

var campaignsCmd = &cobra.Command{
	Use:   "campaigns",
	Short: "List 10DLC campaigns on this account",
	Long:  "Lists all 10DLC campaigns with their registration status, brand, and phone number associations.",
	Example: `  # List all campaigns
  band tendlc campaigns

  # Paginate through results
  band tendlc campaigns --limit 10 --offset 20`,
	RunE: runCampaigns,
}

func runCampaigns(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v2/accounts/%s/tendlc/campaigns?limit=%d&offset=%d",
		acctID, campaignsLimit, campaignsOffset)

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return roleGateError(err, "Campaign Management")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, extractData(result))
}
