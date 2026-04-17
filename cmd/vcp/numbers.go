package vcp

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(numbersCmd)
}

var numbersCmd = &cobra.Command{
	Use:   "numbers <vcp-id>",
	Short: "List phone numbers assigned to a VCP",
	Args:  cobra.ExactArgs(1),
	RunE:  runNumbers,
}

func runNumbers(cmd *cobra.Command, args []string) error {
	if err := cmdutil.ValidateID(args[0]); err != nil {
		return err
	}

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	params := url.Values{}
	params.Set("voiceConfigurationPackageId", args[0])

	var result interface{}
	if err := client.Get(fmt.Sprintf("/v2/accounts/%s/phoneNumbers/voice?%s", acctID, params.Encode()), &result); err != nil {
		return fmt.Errorf("listing VCP numbers: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, result)
}
