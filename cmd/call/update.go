package call

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var updateRedirectURL string

func init() {
	updateCmd.Flags().StringVar(&updateRedirectURL, "redirect-url", "", "URL to redirect the call to (required)")
	_ = updateCmd.MarkFlagRequired("redirect-url")
	Cmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update [callId]",
	Short: "Redirect an active call to a new URL",
	Args:  cobra.ExactArgs(1),
	RunE:  runUpdate,
}

func runUpdate(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}
	client, acctID, err := cmdutil.VoiceClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	reqBody := map[string]string{
		"state":       "active",
		"redirectUrl": updateRedirectURL,
	}

	var result interface{}
	if err := client.Post(fmt.Sprintf("/accounts/%s/calls/%s", acctID, url.PathEscape(args[0])), reqBody, &result); err != nil {
		return fmt.Errorf("updating call: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
