package recording

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list <callId>",
	Short: "List recordings for a call",
	Args:  cobra.ExactArgs(1),
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}
	client, acctID, err := cmdutil.VoiceClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(fmt.Sprintf("/accounts/%s/calls/%s/recordings", acctID, url.PathEscape(args[0])), &result); err != nil {
		return fmt.Errorf("listing recordings: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, result)
}
