package app

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(peersCmd)
}

var peersCmd = &cobra.Command{
	Use:   "peers [app-id]",
	Short: "List SIP peers (locations) associated with an application",
	Args:  cobra.ExactArgs(1),
	RunE:  runPeers,
}

func runPeers(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	path := fmt.Sprintf("/accounts/%s/applications/%s/associatedsippeers", acctID, url.PathEscape(args[0]))
	if err := client.Get(path, &result); err != nil {
		return fmt.Errorf("getting application peers: %w", err)
	}

	// The XML API returns an empty string when there are no peers.
	// Normalize to an empty array for consistent output.
	if s, ok := result.(string); ok && s == "" {
		result = []interface{}{}
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
