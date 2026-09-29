package account

import (
	"bufio"
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
	registerPhone     string
	registerEmail     string
	registerFirstName string
	registerLastName  string
	registerAcceptTOS bool
	registerSmsOptIn  bool
)

const tosURL = "https://www.bandwidth.com/legal/build-terms-of-service/"

// registrationClient is a swappable ClientFunc seam for tests (see cmdutil.VoiceClient); accountIDOverride is unused — no account exists yet.
var registrationClient cmdutil.ClientFunc = func(string) (api.Requester, string, error) {
	return api.NewClientNoAuth(cmdutil.RegistrationHost()), "", nil
}

func init() {
	registerCmd.Flags().StringVar(&registerPhone, "phone", "", "Phone number (required)")
	registerCmd.Flags().StringVar(&registerEmail, "email", "", "Email address (required)")
	registerCmd.Flags().StringVar(&registerFirstName, "first-name", "", "First name (required)")
	registerCmd.Flags().StringVar(&registerLastName, "last-name", "", "Last name (required)")
	registerCmd.Flags().BoolVar(&registerAcceptTOS, "accept-tos", false, "Accept the Build Terms of Service (required; use for non-interactive mode)")
	registerCmd.Flags().BoolVar(&registerSmsOptIn, "sms-opt-in", false, "Opt in to marketing SMS/communications from Bandwidth (optional; independent of MFA delivery consent)")
	_ = registerCmd.MarkFlagRequired("phone")
	_ = registerCmd.MarkFlagRequired("email")
	_ = registerCmd.MarkFlagRequired("first-name")
	_ = registerCmd.MarkFlagRequired("last-name")
	Cmd.AddCommand(registerCmd)
}

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Create a new Bandwidth Build account",
	Long: `Creates a new Bandwidth Build account.

After registration, verify your phone number and complete setup:
  1. band account send-code --phone <phone> --email <email> --delivery-channel sms   (or "voice")
  2. band account verify --phone <phone> --email <email> --code <code-you-received>
  3. Check your email for a registration link from Bandwidth to set your password
  4. Go to Account > API Credentials to generate OAuth2 credentials
  5. Run "band auth login" with those credentials

--sms-opt-in records consent to marketing/PFT-campaign SMS. It is independent
of the MFA delivery consent implied by choosing "sms" as the delivery channel
on "band account send-code" — omit it (or pass --sms-opt-in=false) for no
marketing consent; registration succeeds either way.`,
	Example: `  band account register --phone +19195551234 --email user@example.com --first-name John --last-name Doe --sms-opt-in`,
	RunE:    runRegister,
}

func runRegister(cmd *cobra.Command, args []string) error {
	accepted := registerAcceptTOS

	if !accepted {
		if !cmdutil.IsInteractive() {
			return fmt.Errorf("you must accept the Bandwidth Build Terms of Service to register\n\n"+
				"Review the terms at: %s\n"+
				"Then re-run with --accept-tos", tosURL)
		}

		fmt.Fprintln(os.Stderr)
		ui.Headerf("Bandwidth Build Terms of Service")
		ui.Infof("Before registering, please review the Bandwidth Build Terms of Service:")
		fmt.Fprintf(os.Stderr, "\n  %s\n\n", tosURL)

		fmt.Fprint(os.Stderr, "Do you accept the Build Terms of Service? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer == "y" || answer == "yes" {
			accepted = true
		}
	}

	if !accepted {
		return fmt.Errorf("registration cancelled — you must accept the Build Terms of Service to proceed")
	}

	client, _, err := registrationClient("")
	if err != nil {
		return err
	}

	reqBody := map[string]interface{}{
		"phoneNumber":              registerPhone,
		"email":                    registerEmail,
		"firstName":                registerFirstName,
		"lastName":                 registerLastName,
		"tosAccepted":              true,
		"promotionalCommsAccepted": registerSmsOptIn,
	}

	var result interface{}
	if err := client.Post(cmd.Context(), "/registration", reqBody, &result); err != nil {
		return fmt.Errorf("registering account: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if err := output.StdoutAuto(format, plain, result); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr)
	ui.Successf("Registration submitted!")
	ui.Headerf("Next steps:")
	ui.Infof("1. Verify your phone number:")
	ui.Infof("     band account send-code --phone %s --email %s --delivery-channel sms", registerPhone, registerEmail)
	ui.Infof("     band account verify --phone %s --email %s --code <code-you-received>", registerPhone, registerEmail)
	ui.Infof("   (pass --delivery-channel voice on send-code for a phone call instead of a text)")
	ui.Infof("2. Check your email (%s) for a registration link from Bandwidth to set your password", registerEmail)
	ui.Infof("3. Go to Account > API Credentials to generate your OAuth2 credentials")
	ui.Infof("4. Run: band auth login --client-id <id> --client-secret <secret>")

	return nil
}
