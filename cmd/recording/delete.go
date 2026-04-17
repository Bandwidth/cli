package recording

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

func init() {
	Cmd.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:   "delete <callId> <recordingId>",
	Short: "Delete a recording",
	Args:  cobra.ExactArgs(2),
	RunE:  runDelete,
}

func runDelete(cmd *cobra.Command, args []string) error {
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

	if err := client.Delete(fmt.Sprintf("/accounts/%s/calls/%s/recordings/%s", acctID, url.PathEscape(args[0]), url.PathEscape(args[1])), nil); err != nil {
		return fmt.Errorf("deleting recording: %w", err)
	}

	fmt.Printf("Recording %s deleted.\n", args[1])
	return nil
}
