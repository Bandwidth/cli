package recording

import (
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

var downloadOutput string

func init() {
	downloadCmd.Flags().StringVar(&downloadOutput, "output", "", "File path to write the recording to (required)")
	_ = downloadCmd.MarkFlagRequired("output")
	Cmd.AddCommand(downloadCmd)
}

var downloadCmd = &cobra.Command{
	Use:   "download <callId> <recordingId>",
	Short: "Download a recording to a file",
	Args:  cobra.ExactArgs(2),
	RunE:  runDownload,
}

func runDownload(cmd *cobra.Command, args []string) error {
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

	data, err := client.GetRaw(fmt.Sprintf("/accounts/%s/calls/%s/recordings/%s/media", acctID, url.PathEscape(args[0]), url.PathEscape(args[1])))
	if err != nil {
		return fmt.Errorf("downloading recording: %w", err)
	}

	if err := os.WriteFile(downloadOutput, data, 0644); err != nil {
		return fmt.Errorf("writing recording to file: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Recording saved to %s\n", downloadOutput)
	return nil
}
