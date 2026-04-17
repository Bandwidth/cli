package vcp

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	createName        string
	createDescription string
	createAppID       string
	createIfNotExists bool
)

func init() {
	Cmd.AddCommand(createCmd)
	createCmd.Flags().StringVar(&createName, "name", "", "VCP name (required)")
	createCmd.Flags().StringVar(&createDescription, "description", "", "VCP description")
	createCmd.Flags().StringVar(&createAppID, "app-id", "", "Voice application ID to link")
	createCmd.Flags().BoolVar(&createIfNotExists, "if-not-exists", false, "Return existing VCP if one with the same name exists")
	createCmd.MarkFlagRequired("name")
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a Voice Configuration Package",
	Long:  "Creates a Voice Configuration Package for the Universal Platform. VCPs define voice routing and settings for groups of phone numbers. Link a voice application with --app-id to enable HTTP voice callbacks.",
	Example: `  # Create a basic VCP
  band vcp create --name "Production VCP"

  # Create linked to a voice app
  band vcp create --name "Voice VCP" --app-id abc-123-def

  # Idempotent create (safe for retries)
  band vcp create --name "Voice VCP" --if-not-exists`,
	RunE: runCreate,
}

// VCPCreateOpts holds the parameters for creating a VCP.
type VCPCreateOpts struct {
	Name        string
	Description string
	AppID       string
}

// BuildVCPCreateBody builds the JSON request body for creating a VCP.
func BuildVCPCreateBody(opts VCPCreateOpts) map[string]interface{} {
	body := map[string]interface{}{
		"name": opts.Name,
	}
	if opts.Description != "" {
		body["description"] = opts.Description
	}
	if opts.AppID != "" {
		body["httpVoiceV2ApplicationId"] = opts.AppID
	}
	return body
}

func runCreate(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	format, plain := cmdutil.OutputFlags(cmd)

	if createIfNotExists {
		var listResult interface{}
		if err := client.Get(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages", acctID), &listResult); err == nil {
			if existing := output.FindByName(listResult, "name", createName); existing != nil {
				return output.StdoutAuto(format, plain, existing)
			}
		}
	}

	body := BuildVCPCreateBody(VCPCreateOpts{
		Name:        createName,
		Description: createDescription,
		AppID:       createAppID,
	})

	var result interface{}
	if err := client.Post(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages", acctID), body, &result); err != nil {
		return fmt.Errorf("creating VCP: %w", err)
	}

	return output.StdoutAuto(format, plain, result)
}

