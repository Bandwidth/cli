package app

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
	Short: "List all applications",
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(fmt.Sprintf("/accounts/%s/applications", acctID), &result); err != nil {
		return fmt.Errorf("listing applications: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, result)
}
