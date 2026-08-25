package vcp

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List Voice Configuration Packages",
	Long:  "Lists all Voice Configuration Packages on the account.",
	Example: `  band vcp list
  band vcp list --plain`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(cmd.Context(), fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages", acctID), &result); err != nil {
		return cmdutil.Wrap403(err, "listing VCPs", "VCP")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, result)
}
