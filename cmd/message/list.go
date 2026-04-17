package message

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	listTo        string
	listFrom      string
	listStartDate string
	listEndDate   string
)

func init() {
	listCmd.Flags().StringVar(&listTo, "to", "", "Filter by recipient phone number")
	listCmd.Flags().StringVar(&listFrom, "from", "", "Filter by sender phone number")
	listCmd.Flags().StringVar(&listStartDate, "start-date", "", "Filter messages after this date (e.g. 2024-01-01T00:00:00.000Z)")
	listCmd.Flags().StringVar(&listEndDate, "end-date", "", "Filter messages before this date (e.g. 2024-01-31T23:59:59.000Z)")
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List messages with optional filters",
	Long:  "Lists message metadata with optional filters by recipient, sender, and date range. Note: Bandwidth does not store message content — only metadata is returned.",
	Example: `  # Filter by sender
  band message list --from +15559876543

  # Filter by date range
  band message list --start-date 2024-01-01T00:00:00.000Z --end-date 2024-01-31T23:59:59.000Z`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.MessagingClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	// Build query string from filters
	// The Bandwidth messaging search API uses sourceTn/destinationTn, not from/to.
	// Phone numbers must be URL-encoded (the + sign becomes %2B).
	var params []string
	if listTo != "" {
		params = append(params, "destinationTn="+url.QueryEscape(listTo))
	}
	if listFrom != "" {
		params = append(params, "sourceTn="+url.QueryEscape(listFrom))
	}
	if listStartDate != "" {
		params = append(params, "fromDateTime="+listStartDate)
	}
	if listEndDate != "" {
		params = append(params, "toDateTime="+listEndDate)
	}

	path := fmt.Sprintf("/users/%s/messages", acctID)
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return fmt.Errorf("listing messages: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, extractMessages(result))
}

// extractMessages unwraps the Bandwidth messaging search response to return
// just the messages array. The raw API response deserializes as either:
//   - a map with a "messages" key, or
//   - an array whose first element is such a map.
//
// If the structure doesn't match, the original result is returned as-is.
func extractMessages(result interface{}) interface{} {
	// Try direct map with "messages" key.
	if m, ok := result.(map[string]interface{}); ok {
		if msgs, exists := m["messages"]; exists {
			return msgs
		}
		return result
	}

	// Try array wrapping a map with "messages" key.
	if arr, ok := result.([]interface{}); ok && len(arr) > 0 {
		if m, ok := arr[0].(map[string]interface{}); ok {
			if msgs, exists := m["messages"]; exists {
				return msgs
			}
		}
	}

	return result
}
