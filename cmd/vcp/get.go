package vcp

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get <vcp-id>",
	Short: "Get a Voice Configuration Package",
	Args:  cobra.ExactArgs(1),
	RunE:  runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages/%s", acctID, url.PathEscape(args[0])), &result); err != nil {
		return fmt.Errorf("getting VCP: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
