package quickstart

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	numbercmd "github.com/Bandwidth/cli/cmd/number"
	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	"github.com/Bandwidth/cli/internal/ui"
)

// assignErrIsRetryable reports whether a failed VCP number-assignment is worth
// retrying. The just-ordered number provisions asynchronously, so the bulk
// assign returns VCS-0044 (HTTP 400) until it's ready — that, plus rate limits,
// 5xx, and transport errors, are transient. Auth/validation/not-found errors
// (other 4xx) are not retryable and should fail fast.
func assignErrIsRetryable(err error) bool {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return true // transport/unknown error — treat as transient
	}
	if apiErr.StatusCode == 429 || apiErr.StatusCode == 422 || apiErr.StatusCode >= 500 {
		return true
	}
	return apiErr.StatusCode == 400 && strings.Contains(apiErr.Error(), "VCS-0044")
}

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
is on the legacy platform, use --legacy for the sub-account/location path.

Re-running quickstart is safe on the default (VCP) path: existing resources
are reused and a second number is never ordered. NOTE: re-running --legacy
may order an additional paid number because legacy number ordering is not
idempotent; prefer the default VCP path, which is.`,
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

	// Step 1: Create voice application (idempotent: reuse if already exists)
	appID, err := ensureVoiceApp(dashClient, acctID, qsName+" App", qsCallbackURL)
	if err != nil {
		// App provisioning failing often means this is a legacy account.
		fmt.Fprintf(os.Stderr, "\nVoice application setup failed. If this is a legacy account, try:\n")
		fmt.Fprintf(os.Stderr, "  band quickstart --callback-url %s --legacy\n\n", qsCallbackURL)
		return failWithPartial(result, err)
	}
	result.AppID = appID

	// Step 2: Create VCP linked to the app (idempotent: reuse if already exists)
	vcpName := qsName + " VCP"
	existingVCP, err := findExistingID(platClient, fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages", acctID), "name", vcpName, "voiceConfigurationPackageId")
	if err != nil {
		return failWithPartial(result, err)
	}
	result.VCPID = existingVCP
	if result.VCPID != "" {
		ui.Successf("VCP (existing): %s", ui.ID(result.VCPID))
	} else {
		vcpSpin := ui.NewSpinner("Creating Voice Configuration Package...")
		vcpSpin.Start()
		var vcpResp interface{}
		vcpBody := map[string]interface{}{
			"name":                     vcpName,
			"httpVoiceV2ApplicationId": appID,
		}
		vcpErr := platClient.Post(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages", acctID), vcpBody, &vcpResp)
		vcpSpin.Stop()
		if vcpErr != nil {
			fmt.Fprintf(os.Stderr, "\nVCP creation failed. If this is a legacy account, try:\n")
			fmt.Fprintf(os.Stderr, "  band quickstart --callback-url %s --legacy\n\n", qsCallbackURL)
			return failWithPartial(result, fmt.Errorf("creating VCP: %w", vcpErr))
		}
		result.VCPID = extractIDFromResponse(vcpResp, "voiceConfigurationPackageId")
		ui.Successf("VCP: %s", ui.ID(result.VCPID))
	}
	vcpID := result.VCPID

	// Step 3: Search and order a number (idempotent: skip if VCP already has one)
	existingNum, err := firstAssignedNumber(platClient, acctID, vcpID)
	if err != nil {
		return failWithPartial(result, err)
	}
	if existingNum != "" {
		result.PhoneNumber = existingNum
		result.Status = "complete"
		ui.Successf("Number (existing): %s", ui.ID(existingNum))
	} else {
		// Orders require a sub-account (SiteId), so ensure one exists before ordering.
		siteID, err := ensureSubaccount(dashClient, acctID, qsName+" Sub-account")
		if err != nil {
			return failWithPartial(result, err)
		}
		result.SiteID = siteID
		phoneNumber, err := searchAndOrderNumber(dashClient, acctID, siteID)
		if err != nil {
			result.Status = "complete_no_number"
			ui.Warnf("%v", err)
		} else {
			result.PhoneNumber = phoneNumber
			ui.Successf("Number: %s", ui.ID(phoneNumber))

			// Step 4: Assign number to VCP. The just-ordered number takes a
			// moment to become provisionable for voice (the order is async), so
			// the bulk assign returns VCS-0044 until provisioning catches up.
			// Retry until it succeeds or we time out.
			assignSpin := ui.NewSpinner("Assigning number to VCP...")
			assignSpin.Start()
			assignBody := map[string]interface{}{
				"action":       "ADD",
				"phoneNumbers": []string{phoneNumber},
			}
			var lastAssignErr error
			_, pollErr := cmdutil.Poll(cmdutil.PollConfig{
				Interval: 3 * time.Second,
				Timeout:  90 * time.Second,
				Check: func() (bool, interface{}, error) {
					var assignResp interface{}
					err := platClient.Post(fmt.Sprintf("/v2/accounts/%s/voiceConfigurationPackages/%s/phoneNumbers/bulk", acctID, vcpID), assignBody, &assignResp)
					if err == nil {
						return true, assignResp, nil
					}
					lastAssignErr = err
					if assignErrIsRetryable(err) {
						return false, nil, nil // number still provisioning — keep polling
					}
					return false, nil, err // non-retryable (bad request/auth) — fail fast
				},
			})
			assignSpin.Stop()
			if pollErr != nil {
				result.PhoneNumber = phoneNumber
				// pollErr is ErrPollTimeout on timeout (maps to exit 5) or the
				// fail-fast error otherwise; keep it as %w and surface the last
				// attempt as context. Tell the user how to finish manually.
				return failWithPartial(result, fmt.Errorf("assigning number %s to VCP %s (last attempt: %v) — finish with: band vcp assign %s %s: %w", phoneNumber, vcpID, lastAssignErr, vcpID, phoneNumber, pollErr))
			}
			ui.Successf("Number assigned to VCP")
			result.Status = "complete"
		}
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

	// Step 1: Create sub-account (idempotent: reuse if already exists)
	siteName := qsName + " Sub-account"
	existingSite, err := findExistingID(client, fmt.Sprintf("/accounts/%s/sites", acctID), "Name", siteName, "Id", "id", "siteId")
	if err != nil {
		return failWithPartial(result, err)
	}
	var siteID string
	if existingSite != "" {
		result.SiteID = existingSite
		siteID = existingSite
		ui.Successf("Sub-account (existing): %s", ui.ID(existingSite))
	} else {
		siteSpin := ui.NewSpinner("Creating sub-account...")
		siteSpin.Start()
		var siteResp interface{}
		siteBody := api.XMLBody{
			RootElement: "Site",
			Data:        map[string]interface{}{"Name": siteName},
		}
		siteErr := client.Post(fmt.Sprintf("/accounts/%s/sites", acctID), siteBody, &siteResp)
		siteSpin.Stop()
		if siteErr != nil {
			return failWithPartial(result, fmt.Errorf("creating sub-account: %w", siteErr))
		}
		siteID = extractIDFromResponse(siteResp, "Id", "id", "siteId")
		result.SiteID = siteID
		ui.Successf("Sub-account: %s", ui.ID(siteID))
	}

	// Step 2: Create SIP peer / location (idempotent: reuse if already exists)
	peerName := qsName + " Location"
	existingPeer, err := findExistingID(client, fmt.Sprintf("/accounts/%s/sites/%s/sippeers", acctID, siteID), "PeerName", peerName, "PeerId", "Id", "id")
	if err != nil {
		return failWithPartial(result, err)
	}
	if existingPeer != "" {
		result.SIPPeerID = existingPeer
		ui.Successf("Location (existing): %s", ui.ID(existingPeer))
	} else {
		sipSpin := ui.NewSpinner("Creating location...")
		sipSpin.Start()
		var sipResp interface{}
		sipBody := api.XMLBody{
			RootElement: "SipPeer",
			Data: map[string]interface{}{
				"PeerName":      peerName,
				"IsDefaultPeer": "true",
			},
		}
		sipErr := client.Post(fmt.Sprintf("/accounts/%s/sites/%s/sippeers", acctID, siteID), sipBody, &sipResp)
		sipSpin.Stop()
		if sipErr != nil {
			return failWithPartial(result, fmt.Errorf("creating location: %w", sipErr))
		}
		result.SIPPeerID = extractIDFromResponse(sipResp, "PeerId", "Id", "id")
		ui.Successf("Location: %s", ui.ID(result.SIPPeerID))
	}

	// Step 3: Create voice application (idempotent: reuse if already exists)
	appID, err := ensureVoiceApp(client, acctID, qsName+" App", qsCallbackURL)
	if err != nil {
		return failWithPartial(result, err)
	}
	result.AppID = appID

	// Step 4: Search and order a number.
	// TODO: Legacy number ordering cannot be made idempotent here because there is no
	// sub-account-scoped in-service TN listing endpoint. The account-wide /tns endpoint
	// (used by number.fetchAccountNumbers) would wrongly skip ordering on accounts that
	// already have unrelated numbers assigned to different sub-accounts. Close this TODO
	// if Bandwidth exposes a sub-account-scoped in-service TN endpoint, or if
	// number.fetchAccountNumbers is exported and a heuristic is deemed acceptable.
	ui.Warnf("Note: the legacy number-ordering step is not idempotent — each time you re-run quickstart --legacy, another number may be ordered. The default (VCP) path does not have this limitation.")
	phoneNumber, err := searchAndOrderNumber(client, acctID, siteID)
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

// failWithPartial prints the partial result (so created resource IDs aren't
// lost) and returns the wrapped error. Re-running quickstart reuses those
// resources via the idempotency checks.
func failWithPartial(result quickstartResult, err error) error {
	result.Status = "partial"
	_ = printResult(result)
	return err
}

// findExistingID lists resources at listPath and returns the id of the first
// whose nameField matches name (or "" if none). It FAILS CLOSED: a list error
// is returned to the caller rather than swallowed, because quickstart spends
// money — a transient list failure must NOT cause us to create a duplicate.
func findExistingID(client *api.Client, listPath, nameField, name string, idKeys ...string) (string, error) {
	var resp interface{}
	if err := client.Get(listPath, &resp); err != nil {
		return "", fmt.Errorf("checking for existing resource at %s: %w", listPath, err)
	}
	match := output.FindByName(resp, nameField, name)
	if match == nil {
		return "", nil
	}
	return extractIDFromResponse(match, idKeys...), nil
}

// firstAssignedNumber returns the first phone number already assigned to vcpID,
// reading the explicit `phoneNumber` field rather than sniffing for any numeric
// value. It FAILS CLOSED: a list error is returned, not swallowed, so the caller
// does NOT order a duplicate paid number on a transient failure. The
// voiceConfigurationPackageId filter is honored server-side (verified live), and
// the response shape is {"data":[{"phoneNumber":"+1...", ...}], ...}.
func firstAssignedNumber(client *api.Client, acctID, vcpID string) (string, error) {
	var resp interface{}
	path := fmt.Sprintf("/v2/accounts/%s/phoneNumbers/voice?voiceConfigurationPackageId=%s", acctID, url.QueryEscape(vcpID))
	if err := client.Get(path, &resp); err != nil {
		return "", fmt.Errorf("checking existing VCP numbers for %s: %w", vcpID, err)
	}
	// FlattenResponse unwraps the {data, links, errors, page} envelope to the data array.
	list, ok := output.FlattenResponse(resp).([]interface{})
	if !ok {
		return "", nil
	}
	for _, item := range list {
		if m, ok := item.(map[string]interface{}); ok {
			if pn, ok := m["phoneNumber"].(string); ok && pn != "" {
				return pn, nil
			}
		}
	}
	return "", nil
}

// ensureSubaccount finds-or-creates a sub-account AND a default SIP peer
// (location) in it, returning the site ID. Ordering a number requires both a
// SiteId AND a default SIP peer on that site — without the peer the orders API
// fails with code 5020 ("No default SIP peer is set on the account and site").
// Idempotent: re-running reuses the same named sub-account and location.
func ensureSubaccount(client *api.Client, acctID, name string) (string, error) {
	// Sub-account (site).
	siteID, err := findExistingID(client, fmt.Sprintf("/accounts/%s/sites", acctID), "Name", name, "Id", "id", "siteId")
	if err != nil {
		return "", err
	}
	if siteID != "" {
		ui.Successf("Sub-account (existing): %s", ui.ID(siteID))
	} else {
		spin := ui.NewSpinner("Creating sub-account...")
		spin.Start()
		var resp interface{}
		body := api.XMLBody{RootElement: "Site", Data: map[string]interface{}{"Name": name}}
		err = client.Post(fmt.Sprintf("/accounts/%s/sites", acctID), body, &resp)
		spin.Stop()
		if err != nil {
			return "", fmt.Errorf("creating sub-account: %w", err)
		}
		siteID = extractIDFromResponse(resp, "Id", "id", "siteId")
		ui.Successf("Sub-account: %s", ui.ID(siteID))
	}

	// Default SIP peer (location) — required for ordering (avoids code 5020).
	peerName := name + " Location"
	existingPeer, err := findExistingID(client, fmt.Sprintf("/accounts/%s/sites/%s/sippeers", acctID, siteID), "PeerName", peerName, "PeerId", "Id", "id")
	if err != nil {
		return "", err
	}
	if existingPeer != "" {
		ui.Successf("Location (existing): %s", ui.ID(existingPeer))
	} else {
		spin := ui.NewSpinner("Creating default location...")
		spin.Start()
		var resp interface{}
		body := api.XMLBody{RootElement: "SipPeer", Data: map[string]interface{}{"PeerName": peerName, "IsDefaultPeer": "true"}}
		err = client.Post(fmt.Sprintf("/accounts/%s/sites/%s/sippeers", acctID, siteID), body, &resp)
		spin.Stop()
		if err != nil {
			return "", fmt.Errorf("creating default location: %w", err)
		}
		ui.Successf("Location: %s", ui.ID(extractIDFromResponse(resp, "PeerId", "Id", "id")))
	}
	return siteID, nil
}

// ensureVoiceApp find-or-creates a Voice-V2 application named appName with the
// given callback URL and returns its application ID. Idempotent: re-running
// reuses an existing app with the same name. Shared by both quickstart paths so
// the app payload can't drift between them.
func ensureVoiceApp(client *api.Client, acctID, appName, callbackURL string) (string, error) {
	existing, err := findExistingID(client, fmt.Sprintf("/accounts/%s/applications", acctID), "AppName", appName, "ApplicationId", "applicationId")
	if err != nil {
		return "", err
	}
	if existing != "" {
		ui.Successf("Application (existing): %s", ui.ID(existing))
		return existing, nil
	}
	spin := ui.NewSpinner("Creating voice application...")
	spin.Start()
	var resp interface{}
	body := api.XMLBody{
		RootElement: "Application",
		Data: map[string]interface{}{
			"ServiceType":              "Voice-V2",
			"AppName":                  appName,
			"CallInitiatedCallbackUrl": callbackURL,
		},
	}
	err = client.Post(fmt.Sprintf("/accounts/%s/applications", acctID), body, &resp)
	spin.Stop()
	if err != nil {
		return "", fmt.Errorf("creating voice application: %w", err)
	}
	id := extractIDFromResponse(resp, "ApplicationId", "applicationId")
	ui.Successf("Application: %s", ui.ID(id))
	return id, nil
}

func searchAndOrderNumber(client *api.Client, acctID, siteID string) (string, error) {
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
	// Reuse the shared, live-verified order body (SiteId + ExistingTelephoneNumberOrderType).
	orderBody := api.XMLBody{RootElement: "Order", Data: numbercmd.BuildOrderBody(siteID, []string{phoneNumber})}
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
