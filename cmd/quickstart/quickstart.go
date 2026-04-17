package quickstart

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/ui"
)

var (
	qsCallbackURL string
	qsAreaCode    string
	qsName        string
	qsLegacy      bool
)

// Cmd is the `band quickstart` command.
var Cmd = &cobra.Command{
	Use:   "quickstart",
	Short: "One-command setup: create app, VCP, and order a phone number",
	Long: `Quickstart creates everything you need to make voice calls.

By default, it uses the Universal Platform path (VCP). If your account
is on the legacy platform, use --legacy for the sub-account/location path.`,
	Example: `  # Universal Platform (default)
  band quickstart --callback-url https://example.com/voice

  # Legacy platform
  band quickstart --callback-url https://example.com/voice --legacy

  # Custom area code and name
  band quickstart --callback-url https://example.com/voice --area-code 704 --name "Demo"`,
	RunE: runQuickstart,
}

func init() {
	Cmd.Flags().StringVar(&qsCallbackURL, "callback-url", "", "URL for voice callbacks (required)")
	Cmd.Flags().StringVar(&qsAreaCode, "area-code", "919", "Area code to search for a number")
	Cmd.Flags().StringVar(&qsName, "name", "Quickstart", "Name prefix for created resources")
	Cmd.Flags().BoolVar(&qsLegacy, "legacy", false, "Use legacy sub-account/location provisioning")
	_ = Cmd.MarkFlagRequired("callback-url")
}

type quickstartResult struct {
	Status      string `json:"status"`
	AppID       string `json:"appId,omitempty"`
	VCPID       string `json:"vcpId,omitempty"`
	SiteID      string `json:"siteId,omitempty"`
	SIPPeerID   string `json:"sipPeerId,omitempty"`
	PhoneNumber string `json:"phoneNumber,omitempty"`
	CallbackURL string `json:"callbackUrl"`
	Path        string `json:"path"` // "vcp" or "legacy"
}

func runQuickstart(cmd *cobra.Command, args []string) error {
	if qsLegacy {
		return runLegacyQuickstart(cmd)
	}
	return runVCPQuickstart(cmd)
}

func runVCPQuickstart(cmd *cobra.Command) error {
	// We need both a Dashboard (XML) client for apps and a Platform (JSON) client for VCPs
	dashClient, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}
	platClient, _, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	result := quickstartResult{CallbackURL: qsCallbackURL, Path: "vcp"}

	// Step 1: Create voice application
	appSpin := ui.NewSpinner("Creating voice application...")
	appSpin.Start()
	var appResp interface{}
	appBody := api.XMLBody{
		RootElement: "Application",
		Data: map[string]interface{}{
			"ServiceType":              "Voice-V2",
			"AppName":                  qsName + " App",
			"CallInitiatedCallbackUrl": qsCallbackURL,
		},
	}
	appErr := dashClient.Post(fmt.Sprintf("/accounts/%s/applications", acctID), appBody, &appResp)
	appSpin.Stop()
	if appErr != nil {
		// If voice app creation fails with 409, suggest --legacy
		fmt.Fprintf(os.Stderr, "\nVoice application creation failed. If this is a legacy account, try:\n")
		fmt.Fprintf(os.Stderr, "  band quickstart --callback-url %s --legacy\n\n", qsCallbackURL)
		return fmt.Errorf("creating voice application: %w", appErr)
	}
	appID := extractIDFromResponse(appResp, "ApplicationId", "applicationId")
	result.AppID = appID
	ui.Successf("Application: %s", ui.ID(appID))

	// Step 2: Create VCP linked to the app
	vcpSpin := ui.NewSpinner("Creating Voice Configuration Package...")
	vcpSpin.Start()
	var vcpResp interface{}
	vcpBody := map[string]interface{}{
		"name":                     qsName + " VCP",
		"httpVoiceV2ApplicationId": appID,
	}
	vcpErr := platClient.Post(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages", acctID), vcpBody, &vcpResp)
	vcpSpin.Stop()
	if vcpErr != nil {
		fmt.Fprintf(os.Stderr, "\nVCP creation failed. If this is a legacy account, try:\n")
		fmt.Fprintf(os.Stderr, "  band quickstart --callback-url %s --legacy\n\n", qsCallbackURL)
		return fmt.Errorf("creating VCP: %w", vcpErr)
	}
	vcpID := extractIDFromResponse(vcpResp, "voiceConfigurationPackageId")
	result.VCPID = vcpID
	ui.Successf("VCP: %s", ui.ID(vcpID))

	// Step 3: Search and order a number
	phoneNumber, err := searchAndOrderNumber(dashClient, acctID)
	if err != nil {
		result.Status = "complete_no_number"
		ui.Warnf("%v", err)
	} else {
		result.PhoneNumber = phoneNumber
		ui.Successf("Number: %s", ui.ID(phoneNumber))

		// Step 4: Assign number to VCP
		assignSpin := ui.NewSpinner("Assigning number to VCP...")
		assignSpin.Start()
		assignBody := map[string]interface{}{
			"action":       "ADD",
			"phoneNumbers": []string{phoneNumber},
		}
		var assignResp interface{}
		assignErr := platClient.Post(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages/%s/phoneNumbers/bulk", acctID, vcpID), assignBody, &assignResp)
		assignSpin.Stop()
		if assignErr != nil {
			ui.Warnf("Failed to assign number to VCP: %v", assignErr)
		} else {
			ui.Successf("Number assigned to VCP")
		}

		result.Status = "complete"
	}

	fmt.Fprintln(os.Stderr, "")
	ui.Headerf("Next steps")
	fmt.Fprintf(os.Stderr, "  1. Start your callback server at %s\n", qsCallbackURL)
	if result.PhoneNumber != "" {
		fmt.Fprintf(os.Stderr, "  2. band call create --from %s --to <number> --app-id %s --answer-url %s\n",
			result.PhoneNumber, appID, qsCallbackURL)
	}

	return printResult(result)
}

func runLegacyQuickstart(cmd *cobra.Command) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	result := quickstartResult{CallbackURL: qsCallbackURL, Path: "legacy"}

	// Step 1: Create sub-account
	siteSpin := ui.NewSpinner("Creating sub-account...")
	siteSpin.Start()
	var siteResp interface{}
	siteBody := api.XMLBody{
		RootElement: "Site",
		Data:        map[string]interface{}{"Name": qsName + " Sub-account"},
	}
	siteErr := client.Post(fmt.Sprintf("/accounts/%s/sites", acctID), siteBody, &siteResp)
	siteSpin.Stop()
	if siteErr != nil {
		return fmt.Errorf("creating sub-account: %w", siteErr)
	}
	siteID := extractIDFromResponse(siteResp, "Id", "id", "siteId")
	result.SiteID = siteID
	ui.Successf("Sub-account: %s", ui.ID(siteID))

	// Step 2: Create SIP peer
	sipSpin := ui.NewSpinner("Creating location...")
	sipSpin.Start()
	var sipResp interface{}
	sipBody := api.XMLBody{
		RootElement: "SipPeer",
		Data: map[string]interface{}{
			"PeerName":      qsName + " Location",
			"IsDefaultPeer": "true",
		},
	}
	sipErr := client.Post(fmt.Sprintf("/accounts/%s/sites/%s/sippeers", acctID, siteID), sipBody, &sipResp)
	sipSpin.Stop()
	if sipErr != nil {
		return fmt.Errorf("creating location: %w", sipErr)
	}
	sipPeerID := extractIDFromResponse(sipResp, "PeerId", "Id", "id")
	result.SIPPeerID = sipPeerID
	ui.Successf("Location: %s", ui.ID(sipPeerID))

	// Step 3: Create voice application
	appSpin := ui.NewSpinner("Creating voice application...")
	appSpin.Start()
	var appResp interface{}
	appBody := api.XMLBody{
		RootElement: "Application",
		Data: map[string]interface{}{
			"ServiceType":              "Voice-V2",
			"AppName":                  qsName + " App",
			"CallInitiatedCallbackUrl": qsCallbackURL,
		},
	}
	appErr := client.Post(fmt.Sprintf("/accounts/%s/applications", acctID), appBody, &appResp)
	appSpin.Stop()
	if appErr != nil {
		return fmt.Errorf("creating application: %w", appErr)
	}
	appID := extractIDFromResponse(appResp, "ApplicationId", "applicationId")
	result.AppID = appID
	ui.Successf("Application: %s", ui.ID(appID))

	// Step 4: Search and order a number
	phoneNumber, err := searchAndOrderNumber(client, acctID)
	if err != nil {
		result.Status = "complete_no_number"
		ui.Warnf("%v", err)
	} else {
		result.PhoneNumber = phoneNumber
		result.Status = "complete"
		ui.Successf("Number: %s", ui.ID(phoneNumber))
	}

	fmt.Fprintln(os.Stderr, "")
	ui.Headerf("Next steps")
	fmt.Fprintf(os.Stderr, "  1. Start your callback server at %s\n", qsCallbackURL)
	if result.PhoneNumber != "" {
		fmt.Fprintf(os.Stderr, "  2. band call create --from %s --to <number> --app-id %s --answer-url %s\n",
			result.PhoneNumber, appID, qsCallbackURL)
	}

	return printResult(result)
}

func searchAndOrderNumber(client *api.Client, acctID string) (string, error) {
	searchSpin := ui.NewSpinner(fmt.Sprintf("Searching for number in area code %s...", qsAreaCode))
	searchSpin.Start()
	var searchResp interface{}
	searchErr := client.Get(fmt.Sprintf("/accounts/%s/availableNumbers?areaCode=%s&quantity=1", acctID, qsAreaCode), &searchResp)
	searchSpin.Stop()
	if searchErr != nil {
		return "", fmt.Errorf("number search failed: %w", searchErr)
	}

	phoneNumber := extractPhoneNumber(searchResp)
	if phoneNumber == "" {
		return "", fmt.Errorf("no numbers available in area code %s", qsAreaCode)
	}

	orderSpin := ui.NewSpinner(fmt.Sprintf("Ordering %s...", phoneNumber))
	orderSpin.Start()
	var orderResp interface{}
	orderBody := api.XMLBody{
		RootElement: "Order",
		Data: map[string]interface{}{
			"ExistingTelephoneNumberOrderType": map[string]interface{}{
				"TelephoneNumberList": map[string]interface{}{
					"TelephoneNumber": phoneNumber,
				},
			},
			"SiteId": acctID, // orders need a site ID; for VCP path this may need adjustment
		},
	}
	orderErr := client.Post(fmt.Sprintf("/accounts/%s/orders", acctID), orderBody, &orderResp)
	orderSpin.Stop()
	if orderErr != nil {
		return "", fmt.Errorf("number order failed: %w", orderErr)
	}

	return phoneNumber, nil
}

func printResult(r quickstartResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// extractIDFromResponse walks a response (possibly nested from XML) to find an ID field.
func extractIDFromResponse(resp interface{}, keys ...string) string {
	data, err := json.Marshal(resp)
	if err != nil {
		return ""
	}
	var flat map[string]interface{}
	if err := json.Unmarshal(data, &flat); err != nil {
		return ""
	}
	// Try top level
	for _, k := range keys {
		if v := findInMap(flat, k); v != "" {
			return v
		}
	}
	// Try one level deep (common with data wrapper)
	if d, ok := flat["data"].(map[string]interface{}); ok {
		for _, k := range keys {
			if v := findInMap(d, k); v != "" {
				return v
			}
		}
	}
	return ""
}

func findInMap(m map[string]interface{}, key string) string {
	for k, v := range m {
		if k == key {
			switch val := v.(type) {
			case string:
				if val != "" {
					return val
				}
			case float64:
				return fmt.Sprintf("%.0f", val)
			}
		}
		// Recurse into nested maps
		if nested, ok := v.(map[string]interface{}); ok {
			if found := findInMap(nested, key); found != "" {
				return found
			}
		}
	}
	return ""
}

func extractPhoneNumber(resp interface{}) string {
	data, err := json.Marshal(resp)
	if err != nil {
		return ""
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		var arr []interface{}
		if err := json.Unmarshal(data, &arr); err == nil && len(arr) > 0 {
			if s, ok := arr[0].(string); ok {
				return s
			}
		}
		return ""
	}

	// Walk common shapes
	if found := findInMap(raw, "TelephoneNumber"); found != "" {
		return found
	}

	return ""
}
