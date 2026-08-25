package recording

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
	Use:   "get <callId> <recordingId>",
	Short: "Get metadata for a specific recording",
	Long:  "Returns metadata for a single recording, including its duration, channels, status, and media URL. Use 'recording download' to fetch the audio itself.",
	Example: `  band recording get c-123-abc r-456-def
  band recording get c-123-abc r-456-def --plain`,
	Args: cobra.ExactArgs(2),
	RunE: runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
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
	if err := client.Get(cmd.Context(), fmt.Sprintf("/accounts/%s/calls/%s/recordings/%s", acctID, url.PathEscape(args[0]), url.PathEscape(args[1])), &result); err != nil {
		return fmt.Errorf("getting recording: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
