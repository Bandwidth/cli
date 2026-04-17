package vcp

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
	Use:   "delete <vcp-id>",
	Short: "Delete a Voice Configuration Package",
	Args:  cobra.ExactArgs(1),
	RunE:  runDelete,
}

func runDelete(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	if err := client.Delete(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages/%s", acctID, url.PathEscape(args[0])), nil); err != nil {
		return fmt.Errorf("deleting VCP: %w", err)
	}

	fmt.Printf("VCP %s deleted.\n", args[0])
	return nil
}
