package app

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	createName        string
	createType        string
	createCallbackURL string
	createIfNotExists bool
)

func init() {
	createCmd.Flags().StringVar(&createName, "name", "", "Application name (required)")
	createCmd.Flags().StringVar(&createType, "type", "", "Application type: voice or messaging (required)")
	createCmd.Flags().StringVar(&createCallbackURL, "callback-url", "", "Callback URL (required)")
	createCmd.Flags().BoolVar(&createIfNotExists, "if-not-exists", false, "Return existing application if one with the same name already exists")
	_ = createCmd.MarkFlagRequired("name")
	_ = createCmd.MarkFlagRequired("type")
	_ = createCmd.MarkFlagRequired("callback-url")
	Cmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new application",
	Example: `  # Create a voice application
  band app create --name "My Voice App" --type voice --callback-url https://example.com/voice

  # Create a messaging application
  band app create --name "My SMS App" --type messaging --callback-url https://example.com/sms

  # Idempotent create
  band app create --name "My Voice App" --type voice --callback-url https://example.com/voice --if-not-exists`,
	RunE: runCreate,
}

// CreateOpts holds the parameters for creating an application.
type CreateOpts struct {
	Name        string
	Type        string // "voice" or "messaging"
	CallbackURL string
}

// ValidateCreateOpts validates application creation options.
func ValidateCreateOpts(opts CreateOpts) error {
	if opts.Type != "voice" && opts.Type != "messaging" {
		return fmt.Errorf("--type must be 'voice' or 'messaging', got %q", opts.Type)
	}
	return nil
}

// BuildCreateBody builds the XML request body for creating an application.
func BuildCreateBody(opts CreateOpts) map[string]interface{} {
	if opts.Type == "messaging" {
		return map[string]interface{}{
			"ServiceType":    "Messaging-V2",
			"AppName":        opts.Name,
			"MsgCallbackUrl": opts.CallbackURL,
			"CallbackUrl":    opts.CallbackURL,
		}
	}
	return map[string]interface{}{
		"ServiceType":              "Voice-V2",
		"AppName":                  opts.Name,
		"CallInitiatedCallbackUrl": opts.CallbackURL,
	}
}

func runCreate(cmd *cobra.Command, args []string) error {
	opts := CreateOpts{
		Name:        createName,
		Type:        createType,
		CallbackURL: createCallbackURL,
	}
	if err := ValidateCreateOpts(opts); err != nil {
		return err
	}

	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	format, plain := cmdutil.OutputFlags(cmd)

	if createIfNotExists {
		var listResult interface{}
		if err := client.Get(fmt.Sprintf("/accounts/%s/applications", acctID), &listResult); err != nil {
			return fmt.Errorf("listing applications: %w", err)
		}
		if existing := output.FindByName(listResult, "AppName", createName); existing != nil {
			return output.StdoutAuto(format, plain, existing)
		}
	}

	bodyData := BuildCreateBody(opts)

	var result interface{}
	if err := client.Post(fmt.Sprintf("/accounts/%s/applications", acctID), api.XMLBody{RootElement: "Application", Data: bodyData}, &result); err != nil {
		if strings.Contains(err.Error(), "HTTP voice feature is required") {
			return fmt.Errorf("creating voice application: this account requires the HTTP Voice feature to be enabled.\n"+
				"Contact Bandwidth support to enable it, or check if your account is on the Universal Platform.\n"+
				"If you already have VCPs configured, you may need to link a voice app to them via:\n"+
				"  band vcp create --name <name> --app-id <voice-app-id>")
		}
		return fmt.Errorf("creating application: %w", err)
	}

	return output.StdoutAuto(format, plain, result)
}

