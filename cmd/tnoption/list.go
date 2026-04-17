package tnoption

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	listStatus string
	listTN     string
)

func init() {
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by order status (COMPLETE, FAILED, PARTIAL, PROCESSING)")
	listCmd.Flags().StringVar(&listTN, "tn", "", "Filter by phone number")
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List TN Option Orders",
	Example: `  band tnoption list
  band tnoption list --status COMPLETE
  band tnoption list --tn +19195551234`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	params := url.Values{}
	if listStatus != "" {
		params.Set("status", listStatus)
	}
	if listTN != "" {
		params.Set("tn", stripE164(listTN))
	}

	path := fmt.Sprintf("/accounts/%s/tnoptions", acctID)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return fmt.Errorf("listing TN option orders: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
