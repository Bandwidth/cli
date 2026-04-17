package number

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(getCmd)
}

var getCmd = &cobra.Command{
	Use:   "get <number>",
	Short: "Get voice configuration details for a phone number",
	Long:  "Returns a phone number's voice settings including its Voice Configuration Package assignment. The number must be in E.164 format.",
	Example: `  band number get +19195551234
  band number get +19195551234 --plain`,
	Args: cobra.ExactArgs(1),
	RunE: runGet,
}

func runGet(cmd *cobra.Command, args []string) error {
	phoneNumber := args[0]

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	// The API doesn't support filtering by phone number, so we paginate
	// through all voice numbers and find the match client-side.
	cursor := ""
	for {
		path := fmt.Sprintf("/v2/accounts/%s/phoneNumbers/voice?limit=1000", acctID)
		if cursor != "" {
			path += "&afterCursor=" + cursor
		}

		var raw interface{}
		if err := client.Get(path, &raw); err != nil {
			return fmt.Errorf("getting phone number details: %w", err)
		}

		// Search through the data array for our number.
		if m, ok := raw.(map[string]interface{}); ok {
			if data, ok := m["data"].([]interface{}); ok {
				for _, item := range data {
					if rec, ok := item.(map[string]interface{}); ok {
						if rec["phoneNumber"] == phoneNumber {
							format, plain := cmdutil.OutputFlags(cmd)
							return output.StdoutAuto(format, plain, item)
						}
					}
				}
				// Check for next page.
				if len(data) < 1000 {
					break
				}
			} else {
				break
			}
			if page, ok := m["page"].(map[string]interface{}); ok {
				if next, ok := page["afterCursor"].(string); ok && next != "" {
					cursor = next
					continue
				}
			}
		}
		break
	}

	return fmt.Errorf("phone number %s not found or has no voice configuration", phoneNumber)
}
