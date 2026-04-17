package transcription

import (
	"fmt"
	"net/url"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	createWait    bool
	createTimeout time.Duration
)

func init() {
	createCmd.Flags().BoolVar(&createWait, "wait", false, "Wait until the transcription has content")
	createCmd.Flags().DurationVar(&createTimeout, "timeout", 60*time.Second, "Maximum time to wait (default 60s)")
	Cmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create <callId> <recordingId>",
	Short: "Request a transcription for a recording",
	Args:  cobra.ExactArgs(2),
	RunE:  runCreate,
}

func runCreate(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}
	if err := cmdutil.ValidateID(args[1]); err != nil {
		return err
	}
	client, acctID, err := cmdutil.VoiceClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Post(fmt.Sprintf("/accounts/%s/calls/%s/recordings/%s/transcription", acctID, url.PathEscape(args[0]), url.PathEscape(args[1])), nil, &result); err != nil {
		return fmt.Errorf("creating transcription: %w", err)
	}

	if !createWait {
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, result)
	}

	callID, recordingID := args[0], args[1]
	getPath := fmt.Sprintf("/accounts/%s/calls/%s/recordings/%s/transcription", acctID, url.PathEscape(callID), url.PathEscape(recordingID))

	final, err := cmdutil.Poll(cmdutil.PollConfig{
		Interval: 5 * time.Second,
		Timeout:  createTimeout,
		Check: func() (bool, interface{}, error) {
			var t interface{}
			if err := client.Get(getPath, &t); err != nil {
				return false, nil, fmt.Errorf("polling transcription: %w", err)
			}
			// Consider done when the result is non-nil and has content.
			// The Voice API returns a JSON object with a "transcripts" array.
			m, ok := t.(map[string]interface{})
			if !ok {
				return false, nil, nil
			}
			transcripts, ok := m["transcripts"]
			if !ok {
				return false, nil, nil
			}
			arr, ok := transcripts.([]interface{})
			if !ok || len(arr) == 0 {
				return false, nil, nil
			}
			return true, t, nil
		},
	})
	if err != nil {
		return err
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, final)
}
