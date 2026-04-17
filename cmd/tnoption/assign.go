package tnoption

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	assignCampaignID string
	assignWait       bool
	assignTimeout    time.Duration
)

func init() {
	assignCmd.Flags().StringVar(&assignCampaignID, "campaign-id", "", "10DLC campaign ID to assign (required)")
	assignCmd.Flags().BoolVar(&assignWait, "wait", false, "Wait until the order completes")
	assignCmd.Flags().DurationVar(&assignTimeout, "timeout", 60*time.Second, "Maximum time to wait (default 60s)")
	_ = assignCmd.MarkFlagRequired("campaign-id")
	Cmd.AddCommand(assignCmd)
}

var assignCmd = &cobra.Command{
	Use:   "assign <number> [number...]",
	Short: "Assign phone numbers to a 10DLC campaign",
	Long: `Creates a TN Option Order to assign one or more phone numbers to a 10DLC campaign.

This is the step that connects your phone numbers to an approved campaign so
messages will deliver. Without this, carriers will reject messages with error 4476.`,
	Example: `  band tnoption assign +19195551234 --campaign-id CA3XKE1
  band tnoption assign +19195551234 +19195551235 --campaign-id CA3XKE1 --wait`,
	Args: cobra.MinimumNArgs(1),
	RunE: runAssign,
}

func runAssign(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	// TN Options API wants full E.164 (with + prefix).
	numbers := make([]string, len(args))
	for i, n := range args {
		numbers[i] = cmdutil.NormalizeNumber(n)
	}

	body := map[string]interface{}{
		"TnOptionGroups": map[string]interface{}{
			"TnOptionGroup": map[string]interface{}{
				"A2pSettings": map[string]interface{}{
					"Action":       "asSpecified",
					"CampaignId":   assignCampaignID,
					"MessageClass": "M",
				},
				"TelephoneNumbers": map[string]interface{}{
					"TelephoneNumber": numbers,
				},
			},
		},
	}

	var result interface{}
	if err := client.Post(
		fmt.Sprintf("/accounts/%s/tnoptions", acctID),
		api.XMLBody{RootElement: "TnOptionOrder", Data: body},
		&result,
	); err != nil {
		return fmt.Errorf("creating TN option order: %w", err)
	}

	if !assignWait {
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, result)
	}

	// Extract order ID from response to poll.
	orderID := extractOrderID(result)
	if orderID == "" {
		// Can't poll without an order ID; just return what we got.
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, result)
	}

	final, err := cmdutil.Poll(cmdutil.PollConfig{
		Interval: 2 * time.Second,
		Timeout:  assignTimeout,
		Check: func() (bool, interface{}, error) {
			var orderResult interface{}
			if err := client.Get(
				fmt.Sprintf("/accounts/%s/tnoptions/%s", acctID, orderID),
				&orderResult,
			); err != nil {
				return false, nil, fmt.Errorf("polling TN option order: %w", err)
			}
			status := extractStatus(orderResult)
			switch strings.ToUpper(status) {
			case "COMPLETE":
				return true, orderResult, nil
			case "FAILED", "PARTIAL":
				return true, orderResult, nil
			default:
				return false, nil, nil
			}
		},
	})
	if err != nil {
		return err
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, final)
}

// stripE164 converts "+19195551234" to "9195551234" for the Dashboard API.
func stripE164(number string) string {
	n := strings.TrimPrefix(number, "+")
	if len(n) == 11 && strings.HasPrefix(n, "1") {
		return n[1:]
	}
	return n
}

// extractOrderID digs the order ID out of the API response.
func extractOrderID(result interface{}) string {
	return digString(result, "OrderId")
}

// extractStatus digs the processing status out of the API response.
func extractStatus(result interface{}) string {
	return digString(result, "ProcessingStatus")
}

// digString recursively searches a map for the first occurrence of a key
// and returns its string value.
func digString(v interface{}, key string) string {
	switch val := v.(type) {
	case map[string]interface{}:
		if s, ok := val[key]; ok {
			if str, ok := s.(string); ok {
				return str
			}
		}
		for _, child := range val {
			if found := digString(child, key); found != "" {
				return found
			}
		}
	}
	return ""
}
