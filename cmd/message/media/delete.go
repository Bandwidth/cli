package media

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/ui"
)

func init() {
	Cmd.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:   "delete <mediaId>",
	Short: "Delete a media file",
	Args:  cobra.ExactArgs(1),
	RunE:  runDelete,
}

func runDelete(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}
	client, acctID, err := cmdutil.MessagingClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	if err := client.Delete(fmt.Sprintf("/users/%s/media/%s", acctID, args[0]), nil); err != nil {
		return fmt.Errorf("deleting media: %w", err)
	}

	ui.Successf("Deleted media: %s", args[0])
	return nil
}
