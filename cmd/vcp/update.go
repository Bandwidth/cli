package vcp

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	updateName        string
	updateDescription string
	updateAppID       string
)

func init() {
	Cmd.AddCommand(updateCmd)
	updateCmd.Flags().StringVar(&updateName, "name", "", "VCP name")
	updateCmd.Flags().StringVar(&updateDescription, "description", "", "VCP description")
	updateCmd.Flags().StringVar(&updateAppID, "app-id", "", "Voice application ID to link")
}

var updateCmd = &cobra.Command{
	Use:   "update <vcp-id>",
	Short: "Update a Voice Configuration Package",
	Long:  "Updates an existing Voice Configuration Package. Only the specified fields are changed; omitted fields are left as-is.",
	Example: `  # Rename a VCP
  band vcp update abc-123 --name "New Name"

  # Link a different voice app
  band vcp update abc-123 --app-id def-456

  # Update multiple fields at once
  band vcp update abc-123 --name "Updated" --description "New description" --app-id def-456`,
	Args: cobra.ExactArgs(1),
	RunE: runUpdate,
}

// VCPUpdateOpts holds optional update fields. A nil pointer means "don't change".
type VCPUpdateOpts struct {
	Name        *string
	Description *string
	AppID       *string
}

// BuildVCPUpdateBody builds the PATCH body from update options.
// Returns an error if no fields are set.
func BuildVCPUpdateBody(opts VCPUpdateOpts) (map[string]interface{}, error) {
	body := make(map[string]interface{})
	if opts.Name != nil {
		body["name"] = *opts.Name
	}
	if opts.Description != nil {
		body["description"] = *opts.Description
	}
	if opts.AppID != nil {
		body["httpVoiceV2ApplicationId"] = *opts.AppID
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("at least one flag (--name, --description, or --app-id) must be provided")
	}
	return body, nil
}

func runUpdate(cmd *cobra.Command, args []string) error {
	vcpID := args[0]
	if err := cmdutil.ValidateID(vcpID); err != nil {
		return err
	}

	var opts VCPUpdateOpts
	if cmd.Flags().Changed("name") {
		opts.Name = &updateName
	}
	if cmd.Flags().Changed("description") {
		opts.Description = &updateDescription
	}
	if cmd.Flags().Changed("app-id") {
		opts.AppID = &updateAppID
	}

	body, err := BuildVCPUpdateBody(opts)
	if err != nil {
		return err
	}

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Patch(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages/%s", acctID, vcpID), body, &result); err != nil {
		return fmt.Errorf("updating VCP: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
