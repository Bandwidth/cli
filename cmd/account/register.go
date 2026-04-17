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
)

const tosURL = "https://www.bandwidth.com/legal/build-terms-of-service/"

func init() {
	registerCmd.Flags().StringVar(&registerPhone, "phone", "", "Phone number (required)")
	registerCmd.Flags().StringVar(&registerEmail, "email", "", "Email address (required)")
	registerCmd.Flags().StringVar(&registerFirstName, "first-name", "", "First name (required)")
	registerCmd.Flags().StringVar(&registerLastName, "last-name", "", "Last name (required)")
	registerCmd.Flags().BoolVar(&registerAcceptTOS, "accept-tos", false, "Accept the Build Terms of Service (required; use for non-interactive mode)")
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

After registration, complete account setup in your browser:
  1. Check your email for a registration link from Bandwidth
  2. Enter the OTP code sent via SMS to verify your phone number
  3. Set your password and enter the OTP code from your email
  4. Go to Account > API Credentials to generate OAuth2 credentials
  5. Run "band auth login" with those credentials`,
	Example: `  band account register --phone +19195551234 --email user@example.com --first-name John --last-name Doe`,
	RunE: runRegister,
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

	client := api.NewClientNoAuth("https://api.bandwidth.com/v1/express")

	reqBody := map[string]interface{}{
		"phoneNumber": registerPhone,
		"email":       registerEmail,
		"firstName":   registerFirstName,
		"lastName":    registerLastName,
		"tosAccepted": true,
	}

	var result interface{}
	if err := client.Post("/registration", reqBody, &result); err != nil {
		return fmt.Errorf("registering account: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if err := output.StdoutAuto(format, plain, result); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr)
	ui.Successf("Registration submitted!")
	ui.Headerf("Next steps (complete in your browser):")
	ui.Infof("1. Check your email (%s) for a registration link from Bandwidth", registerEmail)
	ui.Infof("2. Enter the OTP code sent via SMS to %s", registerPhone)
	ui.Infof("3. Set your password and enter the OTP code from your email")
	ui.Infof("4. Go to Account > API Credentials to generate your OAuth2 credentials")
	ui.Infof("5. Run: band auth login --client-id <id> --client-secret <secret>")

	return nil
}
