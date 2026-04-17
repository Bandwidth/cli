package media

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

var getOutput string

func init() {
	getCmd.Flags().StringVar(&getOutput, "output", "", "File path to write the media to (required)")
	_ = getCmd.MarkFlagRequired("output")
	Cmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get <mediaId>",
	Short: "Download a media file",
	Args:  cobra.ExactArgs(1),
	RunE:  runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}
	client, acctID, err := cmdutil.MessagingClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	data, err := client.GetRaw(fmt.Sprintf("/users/%s/media/%s", acctID, args[0]))
	if err != nil {
		return fmt.Errorf("downloading media: %w", err)
	}

	if err := os.WriteFile(getOutput, data, 0644); err != nil {
		return fmt.Errorf("writing media to file: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Media saved to %s\n", getOutput)
	return nil
}
