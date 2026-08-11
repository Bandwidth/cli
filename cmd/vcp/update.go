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

	updateRouteEndpoint     string
	updateRouteEndpointType string
	updateRouteName         string
	updateRoutePlanJSON     string
	updateReplaceRoutes     bool
)

func init() {
	Cmd.AddCommand(updateCmd)
	updateCmd.Flags().StringVar(&updateName, "name", "", "VCP name")
	updateCmd.Flags().StringVar(&updateDescription, "description", "", "VCP description")
	updateCmd.Flags().StringVar(&updateAppID, "app-id", "", "Voice application ID to link")
	updateCmd.Flags().StringVar(&updateRouteEndpoint, "route-endpoint", "", "Origination route endpoint (host, SIP URI, IPv4, or E.164)")
	updateCmd.Flags().StringVar(&updateRouteEndpointType, "route-endpoint-type", "", "Endpoint type: TN, SIP, IP_V4, or FQDN")
	updateCmd.Flags().StringVar(&updateRouteName, "route-name", "", "Route name (default \"primary route\")")
	updateCmd.Flags().StringVar(&updateRoutePlanJSON, "route-plan-json", "", "Full originationRoutePlan object as JSON, or @file")
	updateCmd.Flags().BoolVar(&updateReplaceRoutes, "replace-routes", false, "Confirm replacement of an existing origination route plan. This is a read-then-write check and is best-effort against concurrent modification.")
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
  band vcp update abc-123 --name "Updated" --description "New description" --app-id def-456

  # Replace a VCP's origination route plan
  band vcp update abc-123 --route-endpoint vapi.example.sip.vapi.ai --route-endpoint-type FQDN --replace-routes`,
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

	plan, err := resolveRoutePlan(updateRouteEndpoint, updateRouteEndpointType, updateRouteName, updateRoutePlanJSON)
	if err != nil {
		return err
	}

	body, err := BuildVCPUpdateBody(opts)
	if err != nil {
		// A route-only update (no --name/--description/--app-id) is valid;
		// BuildVCPUpdateBody only knows about the non-route fields.
		if plan == nil {
			return err
		}
		body = make(map[string]interface{})
	}

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	if plan != nil {
		var current map[string]interface{}
		if err := client.Get(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages/%s", acctID, vcpID), &current); err != nil {
			return fmt.Errorf("reading current VCP: %w", err)
		}
		existingPlan := current["originationRoutePlan"]
		if data, ok := current["data"].(map[string]interface{}); ok {
			existingPlan = data["originationRoutePlan"]
		}
		// Writing a route plan replaces the whole plan. Require explicit consent
		// whenever a non-empty plan would be overwritten with something different.
		// This is a read-then-write check, so it is best-effort against
		// concurrent modification of the VCP between the GET and the PATCH.
		if requiresRouteReplaceConfirmation(existingPlan, plan, updateReplaceRoutes) {
			return &cmdutil.ConflictError{Message: fmt.Sprintf(
				"VCP %s already has an origination route plan; writing a new one replaces it — re-run with --replace-routes to confirm", vcpID)}
		}
		body["originationRoutePlan"] = plan
	}

	var result interface{}
	if err := client.Patch(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages/%s", acctID, vcpID), body, &result); err != nil {
		return fmt.Errorf("updating VCP: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, result)
}
