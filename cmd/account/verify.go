package account

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	"github.com/Bandwidth/cli/internal/ui"
)

var (
	verifyPhone string
	verifyEmail string
	verifyCode  string
)

func init() {
	verifyCmd.Flags().StringVar(&verifyPhone, "phone", "", "Phone number in E.164 format being verified (required)")
	verifyCmd.Flags().StringVar(&verifyEmail, "email", "", "Email address used during registration (required)")
	verifyCmd.Flags().StringVar(&verifyCode, "code", "", "6-digit verification code received via SMS or voice call (required)")
	_ = verifyCmd.MarkFlagRequired("phone")
	_ = verifyCmd.MarkFlagRequired("email")
	_ = verifyCmd.MarkFlagRequired("code")
	Cmd.AddCommand(verifyCmd)
}

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify a phone number with the code from \"band account send-code\"",
	Long: `Validates the verification code sent to a registered phone number.

On success, the registration moves to PHONE_VERIFIED and account
provisioning begins in the background. Setting a password still requires
following the link Bandwidth emails to the registered address — that step
has no CLI equivalent.`,
	Example: `  band account verify --phone +19195551234 --email user@example.com --code 123456`,
	RunE:    runVerify,
}

func runVerify(cmd *cobra.Command, args []string) error {
	client := api.NewClientNoAuth(registrationBaseURL)

	reqBody := map[string]interface{}{
		"phoneNumber": verifyPhone,
		"email":       verifyEmail,
		"code":        verifyCode,
	}

	var result interface{}
	if err := client.Post(cmd.Context(), "/registration/code/verify", reqBody, &result); err != nil {
		return fmt.Errorf("verifying phone number: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if err := output.StdoutAuto(format, plain, result); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr)
	ui.Successf("Phone number verified! Account provisioning has begun.")
	ui.Headerf("Next steps:")
	ui.Infof("1. Check your email (%s) for a registration link from Bandwidth to set your password", verifyEmail)
	ui.Infof("2. Go to Account > API Credentials to generate your OAuth2 credentials")
	ui.Infof("3. Run: band auth login --client-id <id> --client-secret <secret>")

	return nil
}
