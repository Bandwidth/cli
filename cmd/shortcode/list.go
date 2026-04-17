package shortcode

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	listLimit  int
	listOffset int
)

func init() {
	listCmd.Flags().IntVar(&listLimit, "limit", 50, "Page size (max 250)")
	listCmd.Flags().IntVar(&listOffset, "offset", 0, "Pagination offset")
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List short codes on this account",
	Long:  "Lists all short codes registered to the account with their status and carrier activation details.",
	Example: `  band shortcode list
  band shortcode list --plain`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v2/accounts/%s/shortcodes?limit=%d&offset=%d",
		acctID, listLimit, listOffset)

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return shortcodeError(err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, extractData(result))
}

func shortcodeError(err error) error {
	if apiErr, ok := err.(*api.APIError); ok && apiErr.StatusCode == 403 {
		return fmt.Errorf("access denied — your credentials may not have short code access.\n"+
			"Contact your Bandwidth account manager to verify")
	}
	return fmt.Errorf("listing short codes: %w", err)
}

func extractData(result interface{}) interface{} {
	m, ok := result.(map[string]interface{})
	if !ok {
		return result
	}
	if data, exists := m["data"]; exists {
		return data
	}
	return result
}
