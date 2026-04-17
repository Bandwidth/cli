package location

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var listSiteID string

func init() {
	listCmd.Flags().StringVar(&listSiteID, "site", "", "Sub-account ID (required)")
	_ = listCmd.MarkFlagRequired("site")
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all locations (SIP peers) under a sub-account",
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	path := fmt.Sprintf("/accounts/%s/sites/%s/sippeers", acctID, listSiteID)
	if err := client.Get(path, &result); err != nil {
		return fmt.Errorf("listing locations: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, result)
}
