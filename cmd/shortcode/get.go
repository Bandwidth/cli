package shortcode

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var getCountry string

func init() {
	getCmd.Flags().StringVar(&getCountry, "country", "USA", "Country code: USA or CAN")
	Cmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get <short-code>",
	Short: "Get short code details and carrier status",
	Long:  "Shows details for a specific short code including per-carrier activation status, lease info, and sub-account/location assignment.",
	Example: `  band shortcode get 12345
  band shortcode get 12345 --country CAN
  band shortcode get 12345 --plain`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v2/accounts/%s/shortcodes/%s/%s",
		acctID, url.PathEscape(args[0]), url.PathEscape(getCountry))

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		if apiErr, ok := err.(*api.APIError); ok {
			switch apiErr.StatusCode {
			case 403:
				return fmt.Errorf("access denied — your credentials may not have short code access.\n"+
					"Contact your Bandwidth account manager to verify")
			case 404:
				return fmt.Errorf("short code %s not found for country %s on this account", args[0], getCountry)
			}
		}
		return fmt.Errorf("getting short code: %w", err)
	}

	// The get endpoint wraps in data array — unwrap to single object
	if data := extractData(result); data != nil {
		if arr, ok := data.([]interface{}); ok && len(arr) == 1 {
			result = arr[0]
		} else {
			result = data
		}
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
