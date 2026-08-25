package call

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
	Short: "List active and recent calls",
	Long:  "Lists calls for the account, including active calls and recently completed ones. Bandwidth retains call records for a limited window, so older calls may not appear.",
	Example: `  band call list
  band call list --plain`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.VoiceClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(cmd.Context(), fmt.Sprintf("/accounts/%s/calls", acctID), &result); err != nil {
		return fmt.Errorf("listing calls: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, result)
}
