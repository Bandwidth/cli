package tfv

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get <phone-number>",
	Short: "Get toll-free verification status",
	Long:  "Shows the verification status and submission details for a toll-free number.",
	Example: `  band tfv get +18005551234
  band tfv get +18005551234 --plain`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
	number := cmdutil.NormalizeNumber(args[0])

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v2/accounts/%s/phoneNumbers/%s/tollFreeVerification",
		acctID, url.PathEscape(number))

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return tfvError(err, number)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}

// tfvError wraps API errors with helpful context for common TFV failure modes.
func tfvError(err error, number string) error {
	apiErr, ok := err.(*api.APIError)
	if !ok {
		return fmt.Errorf("checking verification: %w", err)
	}
	switch apiErr.StatusCode {
	case 403:
		return cmdutil.NewFeatureLimit("access denied — your credentials don't have the TFV role.\n"+
			"Contact your Bandwidth account manager to enable it", err)
	case 404:
		return fmt.Errorf("no verification request found for %s — submit one with: band tfv submit %s",
			number, number)
	default:
		return fmt.Errorf("checking verification: %w", err)
	}
}
