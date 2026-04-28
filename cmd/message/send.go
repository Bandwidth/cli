package message

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	"github.com/Bandwidth/cli/internal/ui"
)

var (
	sendTo         []string
	sendFrom       string
	sendText       string
	sendMedia      []string
	sendAppID      string
	sendTag        string
	sendPriority   string
	sendExpiration string
	sendStdin      bool
)

func init() {
	sendCmd.Flags().StringSliceVar(&sendTo, "to", nil, "Recipient phone number(s) in E.164 format (required, repeatable)")
	sendCmd.Flags().StringVar(&sendFrom, "from", "", "Sender phone number in E.164 format (required)")
	sendCmd.Flags().StringVar(&sendText, "text", "", "Message body text")
	sendCmd.Flags().StringSliceVar(&sendMedia, "media", nil, "Media URL(s) for MMS (repeatable)")
	sendCmd.Flags().StringVar(&sendAppID, "app-id", "", "Bandwidth messaging application ID (required)")
	sendCmd.Flags().StringVar(&sendTag, "tag", "", "Custom tag included in callback events (max 1024 chars)")
	sendCmd.Flags().StringVar(&sendPriority, "priority", "", "Message priority: default or high")
	sendCmd.Flags().StringVar(&sendExpiration, "expiration", "", "Message expiration as RFC-3339 datetime")
	sendCmd.Flags().BoolVar(&sendStdin, "stdin", false, "Read message body from stdin")
	_ = sendCmd.MarkFlagRequired("to")
	_ = sendCmd.MarkFlagRequired("from")
	_ = sendCmd.MarkFlagRequired("app-id")
	Cmd.AddCommand(sendCmd)
}

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send an SMS or MMS message",
	Long:  "Sends an SMS or MMS message. The message is queued for delivery (202 Accepted) — actual delivery status arrives via webhook callbacks on your application.",
	Example: `  # Send an SMS
  band message send --to +15551234567 --from +15559876543 --app-id abc-123 --text "Hello world"

  # Send an MMS with media
  band message send --to +15551234567 --from +15559876543 --app-id abc-123 --text "Check this out" --media https://example.com/image.png

  # Group message
  band message send --to +15551234567,+15552345678 --from +15559876543 --app-id abc-123 --text "Hey everyone"

  # Pipe message body from stdin
  echo "Hello from a script" | band message send --to +15551234567 --from +15559876543 --app-id abc-123 --stdin`,
	RunE: runSend,
}

// SendOpts holds the parameters for sending a message.
type SendOpts struct {
	To          []string
	From        string
	Text        string
	Media       []string
	AppID       string
	Tag         string
	Priority    string
	Expiration  string
}

// ValidateSendOpts validates the send options before making the API call.
func ValidateSendOpts(opts SendOpts) error {
	if opts.Text == "" && len(opts.Media) == 0 {
		return fmt.Errorf("at least one of text or media is required")
	}
	if opts.Priority != "" && opts.Priority != "default" && opts.Priority != "high" {
		return fmt.Errorf("priority must be \"default\" or \"high\"")
	}
	return nil
}

// BuildSendBody builds the request body for sending a message.
func BuildSendBody(opts SendOpts) map[string]interface{} {
	body := map[string]interface{}{
		"to":            opts.To,
		"from":          opts.From,
		"applicationId": opts.AppID,
	}
	if opts.Text != "" {
		body["text"] = opts.Text
	}
	if len(opts.Media) > 0 {
		body["media"] = opts.Media
	}
	if opts.Tag != "" {
		body["tag"] = opts.Tag
	}
	if opts.Priority != "" {
		body["priority"] = opts.Priority
	}
	if opts.Expiration != "" {
		body["expiration"] = opts.Expiration
	}
	return body
}

func runSend(cmd *cobra.Command, args []string) error {
	text := sendText

	// Read from stdin if requested
	if sendStdin {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("--stdin was set but stdin is a terminal — pipe input or use --text instead")
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		text = string(data)
	}

	opts := SendOpts{
		To:         sendTo,
		From:       sendFrom,
		Text:       text,
		Media:      sendMedia,
		AppID:      sendAppID,
		Tag:        sendTag,
		Priority:   sendPriority,
		Expiration: sendExpiration,
	}
	if err := ValidateSendOpts(opts); err != nil {
		return err
	}

	// Build accounts are voice-only — short-circuit before the dashboard
	// preflight, which otherwise flags the pre-provisioned sample callback
	// as a placeholder and points users at an irrelevant fix.
	if cmdutil.ActiveExpress() {
		return cmdutil.NewFeatureLimit(
			"sending messages: Bandwidth Build accounts are voice-only — this requires a full Bandwidth account.\n"+
				"Talk to an expert: https://www.bandwidth.com/talk-to-an-expert/", nil)
	}

	// Preflight: verify the messaging app is linked to a location.
	dashClient, dashAcctID, dashErr := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if dashErr == nil {
		if ok, msg := CheckAppAssociation(dashClient, dashAcctID, sendAppID); !ok {
			return fmt.Errorf("preflight check failed: %s", msg)
		}
		// Block if the callback URL looks fake/missing — without it, delivery
		// failures are invisible and you won't know messages aren't arriving.
		if warning := CheckCallbackURL(dashClient, dashAcctID, sendAppID); warning != "" {
			return fmt.Errorf("preflight check failed: %s", warning)
		}
	}

	// Preflight: verify the from number's provisioning (campaign, TFV, etc.).
	platClient, platAcctID, platErr := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if platErr == nil {
		check := CheckMessagingReadiness(platClient, platAcctID, sendFrom)
		if !check.Ready {
			return fmt.Errorf("preflight check failed: %s", check.Message)
		}
		if check.Message != "" {
			ui.Infof("%s", check.Message)
		}
	}

	client, acctID, err := cmdutil.MessagingClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	reqBody := BuildSendBody(opts)

	var result interface{}
	if err := client.Post(fmt.Sprintf("/users/%s/messages", acctID), reqBody, &result); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
