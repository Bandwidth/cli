package account

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	"github.com/Bandwidth/cli/internal/ui"
)

var (
	sendCodePhone           string
	sendCodeEmail           string
	sendCodeDeliveryChannel string
)

func init() {
	sendCodeCmd.Flags().StringVar(&sendCodePhone, "phone", "", "Phone number in E.164 format, matching a pending registration (required)")
	sendCodeCmd.Flags().StringVar(&sendCodeEmail, "email", "", "Email address used during registration (required)")
	sendCodeCmd.Flags().StringVar(&sendCodeDeliveryChannel, "delivery-channel", "", "Verification code delivery channel: sms or voice (required — the choice itself is the customer's consent to receive the code via that channel)")
	_ = sendCodeCmd.MarkFlagRequired("phone")
	_ = sendCodeCmd.MarkFlagRequired("email")
	_ = sendCodeCmd.MarkFlagRequired("delivery-channel")
	Cmd.AddCommand(sendCodeCmd)
}

var sendCodeCmd = &cobra.Command{
	Use:   "send-code",
	Short: "Send (or resend) a phone verification code for a pending registration",
	Long: `Sends a verification code to the phone number from a pending "band account register" call.

There is no separate MFA-consent flag: choosing "sms" is itself the
customer's consent to receive that one-time code by text message. Choosing
"voice" places a phone call instead and records no such consent. This is
independent of "band account register"'s --sms-opt-in, which is marketing
consent, not verification-code delivery consent.`,
	Example: `  band account send-code --phone +19195551234 --email user@example.com --delivery-channel sms
  band account send-code --phone +19195551234 --email user@example.com --delivery-channel voice`,
	RunE: runSendCode,
}

// normalizeDeliveryChannel upper-cases and validates a --delivery-channel
// value against the wire enum (SMS, VOICE).
func normalizeDeliveryChannel(raw string) (string, error) {
	channel := strings.ToUpper(strings.TrimSpace(raw))
	if channel != "SMS" && channel != "VOICE" {
		return "", cmdutil.NewFlagError("--delivery-channel must be one of: sms, voice")
	}
	return channel, nil
}

func runSendCode(cmd *cobra.Command, args []string) error {
	channel, err := normalizeDeliveryChannel(sendCodeDeliveryChannel)
	if err != nil {
		return err
	}

	client := api.NewClientNoAuth(registrationBaseURL)

	reqBody := map[string]interface{}{
		"phoneNumber":     sendCodePhone,
		"email":           sendCodeEmail,
		"deliveryChannel": channel,
	}

	var result interface{}
	if err := client.Post(cmd.Context(), "/registration/code", reqBody, &result); err != nil {
		return fmt.Errorf("sending verification code: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if err := output.StdoutAuto(format, plain, result); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr)
	ui.Successf("Verification code sent via %s", strings.ToLower(channel))
	ui.Infof("Next: band account verify --phone %s --email %s --code <code-you-received>", sendCodePhone, sendCodeEmail)

	return nil
}
