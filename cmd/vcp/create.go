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

	createRouteEndpoint     string
	createRouteEndpointType string
	createRouteName         string
	createRoutePlanJSON     string
)

func init() {
	Cmd.AddCommand(createCmd)
	createCmd.Flags().StringVar(&createName, "name", "", "VCP name (required)")
	createCmd.Flags().StringVar(&createDescription, "description", "", "VCP description")
	createCmd.Flags().StringVar(&createAppID, "app-id", "", "Voice application ID to link")
	createCmd.Flags().BoolVar(&createIfNotExists, "if-not-exists", false, "Return existing VCP if one with the same name exists")
	createCmd.Flags().StringVar(&createRouteEndpoint, "route-endpoint", "", "Origination route endpoint (host, SIP URI, IPv4, or E.164)")
	createCmd.Flags().StringVar(&createRouteEndpointType, "route-endpoint-type", "", "Endpoint type: TN, SIP, IP_V4, or FQDN")
	createCmd.Flags().StringVar(&createRouteName, "route-name", "", "Route name (default \"primary route\")")
	createCmd.Flags().StringVar(&createRoutePlanJSON, "route-plan-json", "", "Full originationRoutePlan object as JSON, or @file")
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
  band vcp create --name "Voice VCP" --if-not-exists

  # Create with an origination route
  band vcp create --name "Voice VCP" --route-endpoint vapi.example.sip.vapi.ai --route-endpoint-type FQDN`,
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

	plan, err := resolveRoutePlan(createRouteEndpoint, createRouteEndpointType, createRouteName, createRoutePlanJSON)
	if err != nil {
		return err
	}

	if createIfNotExists {
		var listResult interface{}
		if err := client.Get(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages", acctID), &listResult); err == nil {
			matches := findAllByName(listResult, "name", createName)
			if len(matches) > 1 {
				return fmt.Errorf("found %d VCPs named %q; disambiguate by ID with band vcp update <vcp-id>", len(matches), createName)
			}
			if len(matches) == 1 {
				existing := matches[0]
				if err := vcpConflict(existing, createName,
					cmd.Flags().Changed("description"), createDescription,
					cmd.Flags().Changed("app-id"), createAppID,
					plan); err != nil {
					return err
				}
				return output.StdoutAuto(format, plain, existing)
			}
		}
	}

	body := BuildVCPCreateBody(VCPCreateOpts{
		Name:        createName,
		Description: createDescription,
		AppID:       createAppID,
	})
	if plan != nil {
		body["originationRoutePlan"] = plan
	}

	var result interface{}
	if err := client.Post(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages", acctID), body, &result); err != nil {
		return fmt.Errorf("creating VCP: %w", err)
	}

	return output.StdoutAuto(format, plain, result)
}

// findAllByName returns every item in a flattened list response whose field
// matches name. Unlike output.FindByName, callers need the full match count
// to detect ambiguous names before silently acting on the first one found.
func findAllByName(data interface{}, field, name string) []map[string]interface{} {
	flat := output.FlattenResponse(data)
	var matches []map[string]interface{}
	switch v := flat.(type) {
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if val, ok := m[field].(string); ok && val == name {
					matches = append(matches, m)
				}
			}
		}
	case map[string]interface{}:
		if val, ok := v[field].(string); ok && val == name {
			matches = append(matches, v)
		}
	}
	return matches
}

// normalizedField treats a JSON-decoded field value of nil (absent/null) the
// same as an empty string, matching the API's own absent/null equivalence for
// optional scalar fields like description and app ID.
func normalizedField(v interface{}) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// vcpConflict decides whether a single --if-not-exists name match is
// compatible with the requested create, comparing only the fields the caller
// actually specified (checkDescription/checkAppID reflect whether --description
// and --app-id were set; plan is nil unless route flags/--route-plan-json were
// given). Returns nil when the match is compatible (safe to return as-is), or
// an error naming the exact `band vcp update` remediation otherwise. This is
// pulled out of runCreate so the decision can be unit tested without a live
// (or faked) HTTP client.
func vcpConflict(existing map[string]interface{}, name string, checkDescription bool, description string, checkAppID bool, appID string, plan map[string]interface{}) error {
	id := existing["voiceConfigurationPackageId"]
	if checkDescription && normalizedField(existing["description"]) != description {
		return fmt.Errorf("VCP %q exists with a different description — update it explicitly: band vcp update %v --description <value>", name, id)
	}
	if checkAppID && normalizedField(existing["httpVoiceV2ApplicationId"]) != appID {
		return fmt.Errorf("VCP %q exists but is linked to a different application — update it explicitly: band vcp update %v --app-id <value>", name, id)
	}
	if plan != nil && !RoutePlansEqual(existing["originationRoutePlan"], plan) {
		return fmt.Errorf("VCP %q exists with a different origination route plan — update it explicitly: band vcp update %v --route-endpoint ... --route-endpoint-type ... --replace-routes", name, id)
	}
	return nil
}
