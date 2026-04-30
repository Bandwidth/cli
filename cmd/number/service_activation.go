package number

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

// Flags shared by `number activate` and `number deactivate`.
var (
	saVoiceInbound      bool
	saVoiceOutNational  bool
	saVoiceOutInternat  bool
	saMessaging         bool
	saDryRun            bool
	saWait              bool
	saTimeout           time.Duration
	saCustomerOrderID   string
)

// registerServiceActivationFlags wires the shared flag set onto a command.
// Keeps activate and deactivate in lockstep so we can never forget a flag
// on one and not the other.
func registerServiceActivationFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&saVoiceInbound, "voice-inbound", false, "Target the voice INBOUND service")
	cmd.Flags().BoolVar(&saVoiceOutNational, "voice-outbound-national", false, "Target the voice OUTBOUND_NATIONAL service")
	cmd.Flags().BoolVar(&saVoiceOutInternat, "voice-outbound-international", false, "Target the voice OUTBOUND_INTERNATIONAL service")
	cmd.Flags().BoolVar(&saMessaging, "messaging", false, "Target the messaging (ALL) service")
	cmd.Flags().BoolVar(&saDryRun, "dry-run", false, "Run the eligibility checker instead of creating an order")
	cmd.Flags().BoolVar(&saWait, "wait", false, "Block until the order reaches a terminal status")
	cmd.Flags().DurationVar(&saTimeout, "timeout", 60*time.Second, "Maximum time to wait when --wait is set (default 60s)")
	cmd.Flags().StringVar(&saCustomerOrderID, "customer-order-id", "", "Optional customer-supplied order ID for tracking")
}

// ServiceActivationOpts holds the parsed flag state for one invocation.
type ServiceActivationOpts struct {
	Action          string // "ACTIVATE" or "DEACTIVATE"
	PhoneNumbers    []string
	VoiceInbound    bool
	VoiceOutNat     bool
	VoiceOutInt     bool
	Messaging       bool
	CustomerOrderID string
}

// BuildServiceActivationBody constructs the JSON body for
// POST /api/v2/accounts/{accountId}/serviceActivation.
//
// The API requires at least one service to be specified; we surface that
// requirement as a CLI-level validation error rather than letting the API
// reject the request.
func BuildServiceActivationBody(opts ServiceActivationOpts) (map[string]interface{}, error) {
	voice := make([]string, 0, 3)
	if opts.VoiceInbound {
		voice = append(voice, "INBOUND")
	}
	if opts.VoiceOutNat {
		voice = append(voice, "OUTBOUND_NATIONAL")
	}
	if opts.VoiceOutInt {
		voice = append(voice, "OUTBOUND_INTERNATIONAL")
	}

	services := map[string]interface{}{}
	if len(voice) > 0 {
		services["voice"] = voice
	}
	if opts.Messaging {
		services["messaging"] = []string{"ALL"}
	}

	if len(services) == 0 {
		return nil, fmt.Errorf("at least one service flag must be provided: --voice-inbound, --voice-outbound-national, --voice-outbound-international, or --messaging")
	}

	body := map[string]interface{}{
		"action":       opts.Action,
		"phoneNumbers": opts.PhoneNumbers,
		"services":     services,
	}
	if opts.CustomerOrderID != "" {
		body["customerOrderId"] = opts.CustomerOrderID
	}
	return body, nil
}

// BuildCheckerBody constructs the body for the dry-run
// POST /api/v2/accounts/{accountId}/serviceActivationChecker.
func BuildCheckerBody(phoneNumbers []string) map[string]interface{} {
	return map[string]interface{}{"phoneNumbers": phoneNumbers}
}

// runServiceActivation is the shared RunE for activate/deactivate.
func runServiceActivation(cmd *cobra.Command, action string, args []string) error {
	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	// Dry-run path: hit the checker, return the eligibility matrix, done.
	// Service flags are ignored in dry-run mode — the checker reports state
	// for every service regardless of what we asked for.
	if saDryRun {
		body := BuildCheckerBody(args)
		var result interface{}
		path := fmt.Sprintf("/api/v2/accounts/%s/serviceActivationChecker", acctID)
		if err := client.Post(path, body, &result); err != nil {
			return fmt.Errorf("checking service activation: %w", err)
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, result)
	}

	opts := ServiceActivationOpts{
		Action:          action,
		PhoneNumbers:    args,
		VoiceInbound:    saVoiceInbound,
		VoiceOutNat:     saVoiceOutNational,
		VoiceOutInt:     saVoiceOutInternat,
		Messaging:       saMessaging,
		CustomerOrderID: saCustomerOrderID,
	}
	body, err := BuildServiceActivationBody(opts)
	if err != nil {
		return err
	}

	var orderResult map[string]interface{}
	path := fmt.Sprintf("/api/v2/accounts/%s/serviceActivation", acctID)
	if err := client.Post(path, body, &orderResult); err != nil {
		return fmt.Errorf("creating service activation order: %w", err)
	}

	if !saWait {
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, orderResult)
	}

	orderID, ok := extractOrderID(orderResult)
	if !ok {
		return fmt.Errorf("service activation order created but no orderId in response")
	}

	final, err := pollServiceActivationOrder(client, acctID, orderID, saTimeout)
	if err != nil {
		return err
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, final)
}

func extractOrderID(orderResult map[string]interface{}) (string, bool) {
	data, ok := orderResult["data"].(map[string]interface{})
	if !ok {
		return "", false
	}
	orderID, ok := data["orderId"].(string)
	return orderID, ok && orderID != ""
}

// pollServiceActivationOrder polls until the order leaves the in-flight
// states (RECEIVED / PROCESSING) or the timeout fires. We don't enumerate
// terminal states explicitly — anything that's not in-flight is treated
// as terminal so the caller can inspect the final response.
func pollServiceActivationOrder(client *api.Client, acctID, orderID string, timeout time.Duration) (interface{}, error) {
	return cmdutil.Poll(cmdutil.PollConfig{
		Interval: 2 * time.Second,
		Timeout:  timeout,
		Check: func() (bool, interface{}, error) {
			var result map[string]interface{}
			path := fmt.Sprintf("/api/v2/accounts/%s/serviceActivation/%s", acctID, orderID)
			if err := client.Get(path, &result); err != nil {
				return false, nil, fmt.Errorf("polling order %s: %w", orderID, err)
			}
			data, _ := result["data"].(map[string]interface{})
			status, _ := data["orderStatus"].(string)
			switch status {
			case "RECEIVED", "PROCESSING":
				return false, nil, nil
			default:
				return true, result, nil
			}
		},
	})
}
