package recording

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

func init() {
	Cmd.AddCommand(pauseCmd)
}

var pauseCmd = &cobra.Command{
	Use:   "pause <callId>",
	Short: "Pause the active recording on a live call",
	Args:  cobra.ExactArgs(1),
	RunE:  runPause,
}

func runPause(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}
	client, acctID, err := cmdutil.VoiceClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	reqBody := map[string]string{
		"state": "paused",
	}

	if err := client.Put(cmd.Context(), fmt.Sprintf("/accounts/%s/calls/%s/recording", acctID, url.PathEscape(args[0])), reqBody, nil); err != nil {
		return fmt.Errorf("pausing recording: %w", err)
	}

	fmt.Printf("Recording paused on call %s.\n", args[0])
	return nil
}
