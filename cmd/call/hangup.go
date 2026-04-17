package call

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(hangupCmd)
}

var hangupCmd = &cobra.Command{
	Use:   "hangup [callId]",
	Short: "Hang up an active call",
	Args:  cobra.ExactArgs(1),
	RunE:  runHangup,
}

func runHangup(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}
	client, acctID, err := cmdutil.VoiceClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	reqBody := map[string]string{
		"state": "completed",
	}

	var result interface{}
	if err := client.Post(fmt.Sprintf("/accounts/%s/calls/%s", acctID, url.PathEscape(args[0])), reqBody, &result); err != nil {
		return fmt.Errorf("hanging up call: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
