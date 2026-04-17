package tfv

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	submitBusinessName    string
	submitBusinessAddr    string
	submitBusinessCity    string
	submitBusinessState   string
	submitBusinessZip     string
	submitContactFirst    string
	submitContactLast     string
	submitContactEmail    string
	submitContactPhone    string
	submitMessageVolume   int
	submitUseCase         string
	submitUseCaseSummary  string
	submitSampleMessage   string
	submitPrivacyURL      string
	submitTermsURL        string
	submitEntityType      string
)

func init() {
	submitCmd.Flags().StringVar(&submitBusinessName, "business-name", "", "Legal business name (required)")
	submitCmd.Flags().StringVar(&submitBusinessAddr, "business-addr", "", "Business street address (required)")
	submitCmd.Flags().StringVar(&submitBusinessCity, "business-city", "", "Business city (required)")
	submitCmd.Flags().StringVar(&submitBusinessState, "business-state", "", "Business state, 2-letter code (required)")
	submitCmd.Flags().StringVar(&submitBusinessZip, "business-zip", "", "Business postal code (required)")
	submitCmd.Flags().StringVar(&submitContactFirst, "contact-first", "", "Contact first name (required)")
	submitCmd.Flags().StringVar(&submitContactLast, "contact-last", "", "Contact last name (required)")
	submitCmd.Flags().StringVar(&submitContactEmail, "contact-email", "", "Contact email (required)")
	submitCmd.Flags().StringVar(&submitContactPhone, "contact-phone", "", "Contact phone in E.164 (required)")
	submitCmd.Flags().IntVar(&submitMessageVolume, "message-volume", 0, "Estimated monthly message volume (required)")
	submitCmd.Flags().StringVar(&submitUseCase, "use-case", "", "Use case category, e.g. 2FA, MARKETING (required)")
	submitCmd.Flags().StringVar(&submitUseCaseSummary, "use-case-summary", "", "Brief summary of how the number will be used (required)")
	submitCmd.Flags().StringVar(&submitSampleMessage, "sample-message", "", "Example message content (required)")
	submitCmd.Flags().StringVar(&submitPrivacyURL, "privacy-url", "", "Privacy policy URL (required)")
	submitCmd.Flags().StringVar(&submitTermsURL, "terms-url", "", "Terms and conditions URL (required)")
	submitCmd.Flags().StringVar(&submitEntityType, "entity-type", "", "Business entity type: SOLE_PROPRIETOR, PRIVATE_PROFIT, PUBLIC_PROFIT, NON_PROFIT, GOVERNMENT (required)")

	_ = submitCmd.MarkFlagRequired("business-name")
	_ = submitCmd.MarkFlagRequired("business-addr")
	_ = submitCmd.MarkFlagRequired("business-city")
	_ = submitCmd.MarkFlagRequired("business-state")
	_ = submitCmd.MarkFlagRequired("business-zip")
	_ = submitCmd.MarkFlagRequired("contact-first")
	_ = submitCmd.MarkFlagRequired("contact-last")
	_ = submitCmd.MarkFlagRequired("contact-email")
	_ = submitCmd.MarkFlagRequired("contact-phone")
	_ = submitCmd.MarkFlagRequired("message-volume")
	_ = submitCmd.MarkFlagRequired("use-case")
	_ = submitCmd.MarkFlagRequired("use-case-summary")
	_ = submitCmd.MarkFlagRequired("sample-message")
	_ = submitCmd.MarkFlagRequired("privacy-url")
	_ = submitCmd.MarkFlagRequired("terms-url")
	_ = submitCmd.MarkFlagRequired("entity-type")

	Cmd.AddCommand(submitCmd)
}

var submitCmd = &cobra.Command{
	Use:   "submit <phone-number>",
	Short: "Submit a toll-free verification request",
	Long: `Submits a new toll-free verification request. All fields are required by the
carrier ecosystem. Verification is reviewed by carriers and typically takes
a few business days.`,
	Example: `  band tfv submit +18005551234 \
    --business-name "Acme Corp" \
    --business-addr "123 Main St" \
    --business-city "Raleigh" \
    --business-state "NC" \
    --business-zip "27606" \
    --contact-first "Jane" \
    --contact-last "Doe" \
    --contact-email "jane@acme.com" \
    --contact-phone "+19195551234" \
    --message-volume 10000 \
    --use-case "2FA" \
    --use-case-summary "Two-factor authentication codes for user login" \
    --sample-message "Your Acme verification code is 123456" \
    --privacy-url "https://acme.com/privacy" \
    --terms-url "https://acme.com/terms" \
    --entity-type "PRIVATE_PROFIT"`,
	Args: cobra.ExactArgs(1),
	RunE: runSubmit,
}

func runSubmit(cmd *cobra.Command, args []string) error {
	number := cmdutil.NormalizeNumber(args[0])

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	body := map[string]interface{}{
		"submission": map[string]interface{}{
			"businessAddress": map[string]interface{}{
				"name":  submitBusinessName,
				"addr1": submitBusinessAddr,
				"city":  submitBusinessCity,
				"state": submitBusinessState,
				"zip":   submitBusinessZip,
			},
			"businessContact": map[string]interface{}{
				"firstName":   submitContactFirst,
				"lastName":    submitContactLast,
				"email":       submitContactEmail,
				"phoneNumber": submitContactPhone,
			},
			"messageVolume":            submitMessageVolume,
			"useCase":                  submitUseCase,
			"useCaseSummary":           submitUseCaseSummary,
			"productionMessageContent": submitSampleMessage,
			"privacyPolicyUrl":         submitPrivacyURL,
			"termsAndConditionsUrl":    submitTermsURL,
			"businessEntityType":       submitEntityType,
		},
	}

	path := fmt.Sprintf("/api/v2/accounts/%s/phoneNumbers/%s/tollFreeVerification",
		acctID, url.PathEscape(number))

	var result interface{}
	if err := client.Post(path, body, &result); err != nil {
		if apiErr, ok := err.(*api.APIError); ok {
			switch apiErr.StatusCode {
			case 403:
				return fmt.Errorf("access denied — your credentials don't have the TFV role.\n"+
					"Contact your Bandwidth account manager to enable it")
			case 400:
				return fmt.Errorf("validation error: %s", apiErr.Body)
			}
		}
		return fmt.Errorf("submitting verification: %w", err)
	}

	// POST returns 202 with empty body on success
	if result == nil {
		result = map[string]interface{}{
			"status":      "submitted",
			"phoneNumber": number,
			"message":     "Verification request submitted. Check status with: band tfv get " + number,
		}
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
