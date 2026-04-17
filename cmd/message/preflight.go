package message

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
)

// PreflightResult describes whether a number is ready to send messages.
type PreflightResult struct {
	Ready      bool
	NumberType cmdutil.NumberType
	CampaignID string // non-empty if assigned to a 10DLC campaign
	Message    string // human-readable status
}

// CheckCallbackURL verifies that the messaging application has a callback URL
// that looks like a real server. Without one, delivery confirmations are lost.
func CheckCallbackURL(dashClient *api.Client, acctID, appID string) string {
	var result interface{}
	path := fmt.Sprintf("/accounts/%s/applications/%s", acctID, url.PathEscape(appID))
	if err := dashClient.Get(path, &result); err != nil {
		return "" // can't check, don't warn
	}

	callbackURL := findCallbackURL(result)
	if callbackURL == "" || isPlaceholderURL(callbackURL) {
		return fmt.Sprintf("message not sent — your messaging application has no working callback URL.\n"+
			"Without a callback server, delivery failures are invisible and messages can silently disappear.\n"+
			"Set a callback URL: band app update %s --callback-url https://your-server.example.com/callbacks", appID)
	}
	return ""
}

func findCallbackURL(resp interface{}) string {
	data, err := json.Marshal(resp)
	if err != nil {
		return ""
	}
	// Look for MsgCallbackUrl first (messaging apps), then CallbackUrl
	var urls []string
	findFieldValues(data, "MsgCallbackUrl", &urls)
	if len(urls) > 0 && urls[0] != "" {
		return urls[0]
	}
	findFieldValues(data, "CallbackUrl", &urls)
	if len(urls) > 0 {
		return urls[0]
	}
	return ""
}

func isPlaceholderURL(u string) bool {
	placeholders := []string{
		"example.com",
		"localhost",
		"127.0.0.1",
		"google.com",
		"bandwidth.com",
	}
	for _, p := range placeholders {
		if strings.Contains(u, p) {
			return true
		}
	}
	return false
}

// CheckAppAssociation verifies that a messaging application is linked to at
// least one location (SIP peer). If it has no associations, messages sent
// through it will silently vanish — 202 accepted but never delivered.
//
// It checks both the app's associatedsippeers endpoint AND each location's
// applicationSettings (the assignment may only be visible from the location side).
func CheckAppAssociation(dashClient *api.Client, acctID, appID string) (bool, string) {
	// First try the app-level query (fast path)
	var peersResult interface{}
	path := fmt.Sprintf("/accounts/%s/applications/%s/associatedsippeers", acctID, url.PathEscape(appID))
	if err := dashClient.Get(path, &peersResult); err == nil {
		peers := extractAssociatedPeers(peersResult)
		if len(peers) > 0 {
			return true, ""
		}
	}

	// App-level query found nothing — check from the location side.
	// List all sites, then check each location's messaging applicationSettings.
	var sitesResult interface{}
	if err := dashClient.Get(fmt.Sprintf("/accounts/%s/sites", acctID), &sitesResult); err != nil {
		return true, "" // can't check, don't block
	}

	siteIDs := extractSiteIDs(sitesResult)
	for _, siteID := range siteIDs {
		var locsResult interface{}
		if err := dashClient.Get(fmt.Sprintf("/accounts/%s/sites/%s/sippeers", acctID, siteID), &locsResult); err != nil {
			continue
		}
		peerIDs := extractPeerIDs(locsResult)
		for _, peerID := range peerIDs {
			var settings interface{}
			settingsPath := fmt.Sprintf("/accounts/%s/sites/%s/sippeers/%s/products/messaging/applicationSettings", acctID, siteID, peerID)
			if err := dashClient.Get(settingsPath, &settings); err != nil {
				continue
			}
			if foundAppID := extractAppIDFromSettings(settings); foundAppID == appID {
				return true, ""
			}
		}
	}

	return false, fmt.Sprintf("messaging application %s is not linked to any location — messages will silently fail.\n"+
		"Fix: band app assign %s --site <site-id> --location <location-id>\n"+
		"Find IDs: band subaccount list && band location list --site <site-id>", appID, appID)
}

// CheckMessagingReadiness verifies that a phone number is properly provisioned
// for messaging. For 10DLC numbers, it checks campaign assignment via the
// tendlc API. For toll-free and short codes, it returns advisory messages
// since those checks require credentials we may not have.
func CheckMessagingReadiness(platClient *api.Client, acctID, fromNumber string) PreflightResult {
	nt := cmdutil.ClassifyNumber(fromNumber)

	switch nt {
	case cmdutil.NumberType10DLC:
		return check10DLC(platClient, acctID, fromNumber)
	case cmdutil.NumberTypeTollFree:
		return checkTollFree(platClient, acctID, fromNumber)
	case cmdutil.NumberTypeShortCode:
		return PreflightResult{
			Ready:      true, // we can't check, assume provisioned
			NumberType: nt,
			Message:    "short code — check carrier status with: band shortcode get " + fromNumber,
		}
	default:
		return PreflightResult{Ready: true, NumberType: nt}
	}
}

// check10DLC iterates the account's 10DLC campaigns and checks if the number
// is assigned to any of them with SUCCESS status.
func check10DLC(platClient *api.Client, acctID, number string) PreflightResult {
	result := PreflightResult{NumberType: cmdutil.NumberType10DLC}

	// Normalize to E.164 for the filter param
	e164 := number
	if !strings.HasPrefix(e164, "+") {
		e164 = "+" + e164
	}

	// List all campaigns
	var campaignsResp interface{}
	if err := platClient.Get(fmt.Sprintf("/api/v2/accounts/%s/tendlc/campaigns", acctID), &campaignsResp); err != nil {
		// Can't check — don't block the send, just warn
		result.Ready = true
		result.Message = "could not verify campaign assignment (API error) — ensure the number is on an approved campaign"
		return result
	}

	campaigns := extractCampaigns(campaignsResp)
	if len(campaigns) == 0 {
		result.Ready = false
		result.Message = "no 10DLC campaigns found on this account — the number must be assigned to an approved campaign before messages will deliver.\n" +
			"Check registration: band tendlc campaigns"
		return result
	}

	// Check each active campaign for this phone number
	for _, c := range campaigns {
		if c.status != "REGISTERED" {
			continue
		}
		var pnResp interface{}
		path := fmt.Sprintf("/api/v2/accounts/%s/tendlc/campaigns/%s/phoneNumbers?phoneNumber=%s",
			acctID, url.PathEscape(c.id), url.QueryEscape(e164))
		if err := platClient.Get(path, &pnResp); err != nil {
			continue
		}
		if pn := findPhoneNumberInResponse(pnResp, e164); pn != nil {
			if pn.status == "SUCCESS" {
				result.Ready = true
				result.CampaignID = c.id
				result.Message = fmt.Sprintf("number is assigned to campaign %s (status: SUCCESS)", c.id)
				return result
			}
			// Found but not SUCCESS — still provisioning
			result.Ready = false
			result.CampaignID = c.id
			result.Message = fmt.Sprintf("number is on campaign %s but status is %q — it may not be fully provisioned yet", c.id, pn.status)
			return result
		}
	}

	// Not found on any campaign
	result.Ready = false
	result.Message = fmt.Sprintf("number is not assigned to any active 10DLC campaign — delivery will fail (error 4476).\n"+
		"Check registration status: band tendlc number %s\n"+
		"List campaigns: band tendlc campaigns\n"+
		"Assign to a campaign: band tnoption assign %s --campaign-id <campaign-id>", number, number)
	return result
}

func checkTollFree(platClient *api.Client, acctID, number string) PreflightResult {
	result := PreflightResult{NumberType: cmdutil.NumberTypeTollFree}

	e164 := number
	if !strings.HasPrefix(e164, "+") {
		e164 = "+" + e164
	}

	var tfvResp interface{}
	if err := platClient.Get(fmt.Sprintf("/api/v2/accounts/%s/phoneNumbers/%s/tollFreeVerification", acctID, url.PathEscape(e164)), &tfvResp); err != nil {
		// 403 means the credential doesn't have TFV access — don't block, just advise
		if apiErr, ok := err.(*api.APIError); ok && apiErr.StatusCode == 403 {
			result.Ready = true
			result.Message = "toll-free verification status could not be checked (insufficient permissions) — ensure TFV is approved"
			return result
		}
		result.Ready = true
		result.Message = "toll-free verification status could not be checked — ensure TFV is approved before sending"
		return result
	}

	status := extractTFVStatus(tfvResp)
	switch strings.ToUpper(status) {
	case "VERIFIED":
		result.Ready = true
		result.Message = "toll-free verification: VERIFIED"
	case "":
		result.Ready = true
		result.Message = "toll-free verification status unknown — ensure TFV is approved"
	default:
		result.Ready = false
		result.Message = fmt.Sprintf("toll-free verification status is %q — must be VERIFIED before messages will deliver.\n"+
			"Check status: band tfv get %s", status, number)
	}
	return result
}

// --- response parsing helpers ---

type campaignInfo struct {
	id     string
	status string
}

type phoneNumberInfo struct {
	number string
	status string
}

func extractCampaigns(resp interface{}) []campaignInfo {
	data, err := json.Marshal(resp)
	if err != nil {
		return nil
	}
	var parsed struct {
		Data []struct {
			CampaignID string `json:"campaignId"`
			Status     string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	var result []campaignInfo
	for _, c := range parsed.Data {
		result = append(result, campaignInfo{id: c.CampaignID, status: c.Status})
	}
	return result
}

func findPhoneNumberInResponse(resp interface{}, e164 string) *phoneNumberInfo {
	data, err := json.Marshal(resp)
	if err != nil {
		return nil
	}
	var parsed struct {
		Data []struct {
			PhoneNumber string `json:"phoneNumber"`
			Status      string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil
	}
	for _, pn := range parsed.Data {
		if pn.PhoneNumber == e164 {
			return &phoneNumberInfo{number: pn.PhoneNumber, status: pn.Status}
		}
	}
	return nil
}

func extractSiteIDs(resp interface{}) []string {
	data, err := json.Marshal(resp)
	if err != nil {
		return nil
	}
	var ids []string
	findFieldValues(data, "Id", &ids)
	return ids
}

func extractPeerIDs(resp interface{}) []string {
	data, err := json.Marshal(resp)
	if err != nil {
		return nil
	}
	var ids []string
	findFieldValues(data, "PeerId", &ids)
	return ids
}

func extractAppIDFromSettings(resp interface{}) string {
	data, err := json.Marshal(resp)
	if err != nil {
		return ""
	}
	var ids []string
	findFieldValues(data, "HttpMessagingV2AppId", &ids)
	if len(ids) > 0 {
		return ids[0]
	}
	return ""
}

func findFieldValues(data []byte, fieldName string, values *[]string) {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	for k, v := range m {
		if k == fieldName {
			if s, ok := v.(string); ok && s != "" {
				*values = append(*values, s)
			}
		}
		if nested, ok := v.(map[string]interface{}); ok {
			d, _ := json.Marshal(nested)
			findFieldValues(d, fieldName, values)
		}
		if arr, ok := v.([]interface{}); ok {
			for _, item := range arr {
				d, _ := json.Marshal(item)
				findFieldValues(d, fieldName, values)
			}
		}
	}
}

func extractAssociatedPeers(resp interface{}) []string {
	data, err := json.Marshal(resp)
	if err != nil {
		return nil
	}
	// The response is XML-parsed, so it could be nested in various wrapper keys.
	// Look for any PeerId values recursively.
	var ids []string
	findPeerIDs(data, &ids)
	return ids
}

func findPeerIDs(data []byte, ids *[]string) {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	for k, v := range m {
		if k == "PeerId" || k == "peerId" {
			if s, ok := v.(string); ok && s != "" {
				*ids = append(*ids, s)
			}
		}
		// Recurse into nested maps
		if nested, ok := v.(map[string]interface{}); ok {
			d, _ := json.Marshal(nested)
			findPeerIDs(d, ids)
		}
		// Recurse into arrays
		if arr, ok := v.([]interface{}); ok {
			for _, item := range arr {
				d, _ := json.Marshal(item)
				findPeerIDs(d, ids)
			}
		}
	}
}

func extractTFVStatus(resp interface{}) string {
	data, err := json.Marshal(resp)
	if err != nil {
		return ""
	}
	var parsed struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ""
	}
	return parsed.Status
}
