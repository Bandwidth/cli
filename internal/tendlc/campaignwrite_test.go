package tendlc

import (
	"context"
	"testing"
)

func TestCreateCampaignPostsToCampaignsPath(t *testing.T) {
	var got captured
	s := stubService(t, 202, `{"data":{"bandwidthId":"CABC123"}}`, &got)

	env, err := s.CreateCampaign(context.Background(), map[string]any{"campaignName": "Acme Alerts"})
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	if got.method != "POST" {
		t.Errorf("method = %q, want POST", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/campaigns"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	if got.body["campaignName"] != "Acme Alerts" {
		t.Errorf("body campaignName = %v, want Acme Alerts", got.body["campaignName"])
	}
	obj, err := env.Object()
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	if obj["bandwidthId"] != "CABC123" {
		t.Errorf("bandwidthId = %v, want CABC123", obj["bandwidthId"])
	}
}

func TestCreateCampaignSyncBodyCarriesOnlyCampaignID(t *testing.T) {
	var got captured
	s := stubService(t, 202, `{"data":{"bandwidthId":"CABC123"}}`, &got)

	if _, err := s.CreateCampaign(context.Background(), map[string]any{"campaignId": "CEXMPL1"}); err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	if got.method != "POST" {
		t.Errorf("method = %q, want POST", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/campaigns"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	if got.body["campaignId"] != "CEXMPL1" {
		t.Errorf("body campaignId = %v, want CEXMPL1", got.body["campaignId"])
	}
}

func TestUpdateCampaignPutsToCampaignPath(t *testing.T) {
	var got captured
	s := stubService(t, 202, `{"data":{"bandwidthId":"CEXMPL1"}}`, &got)

	if _, err := s.UpdateCampaign(context.Background(), "CEXMPL1", map[string]any{"campaignName": "Acme Alerts"}); err != nil {
		t.Fatalf("UpdateCampaign: %v", err)
	}
	if got.method != "PUT" {
		t.Errorf("method = %q, want PUT", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/campaigns/CEXMPL1"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	if got.body["campaignName"] != "Acme Alerts" {
		t.Errorf("body campaignName = %v, want Acme Alerts", got.body["campaignName"])
	}
}

func TestDeactivateCampaignUsesDelete(t *testing.T) {
	var got captured
	s := stubService(t, 204, "", &got)

	if err := s.DeactivateCampaign(context.Background(), "CEXMPL1"); err != nil {
		t.Fatalf("DeactivateCampaign: %v", err)
	}
	if got.method != "DELETE" {
		t.Errorf("method = %q, want DELETE", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/campaigns/CEXMPL1"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
}

func TestNudgeCampaignPostsToNudgePath(t *testing.T) {
	var got captured
	// 204 with an empty body: NudgeCampaign must not try to parse an
	// envelope out of nothing.
	s := stubService(t, 204, "", &got)

	if err := s.NudgeCampaign(context.Background(), "CEXMPL1", map[string]any{"reason": "retry"}); err != nil {
		t.Fatalf("NudgeCampaign: %v", err)
	}
	if got.method != "POST" {
		t.Errorf("method = %q, want POST", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/campaigns/CEXMPL1/nudge"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	if got.body["reason"] != "retry" {
		t.Errorf("body reason = %v, want retry", got.body["reason"])
	}
}

func TestCampaignPhoneNumbersEncodesPagination(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":[],"page":{"totalElements":0}}`, &got)

	if _, err := s.CampaignPhoneNumbers(context.Background(), "CEXMPL1", 10, 20); err != nil {
		t.Fatalf("CampaignPhoneNumbers: %v", err)
	}
	if want := "/api/v2/accounts/9901287/tendlc/campaigns/CEXMPL1/phoneNumbers"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	if got.query != "limit=10&offset=20" {
		t.Errorf("query = %q, want limit=10&offset=20", got.query)
	}
}

func TestCampaignHistoryEncodesPagination(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":[],"page":{"totalElements":0}}`, &got)

	if _, err := s.CampaignHistory(context.Background(), "CEXMPL1", 10, 20); err != nil {
		t.Fatalf("CampaignHistory: %v", err)
	}
	if want := "/api/v2/accounts/9901287/tendlc/campaigns/CEXMPL1/history"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	if got.query != "limit=10&offset=20" {
		t.Errorf("query = %q, want limit=10&offset=20", got.query)
	}
}

// Every method that takes a campaign ID must reject an empty one before
// making a request. Without this a caller with an unset variable silently
// hits the collection endpoint — DELETE on /campaigns rather than
// /campaigns/{id}.
func TestEmptyCampaignIDsRejectedWithoutRequest(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":{}}`, &got)

	calls := map[string]func() error{
		"UpdateCampaign":     func() error { _, err := s.UpdateCampaign(context.Background(), "", map[string]any{}); return err },
		"DeactivateCampaign": func() error { return s.DeactivateCampaign(context.Background(), "") },
		"NudgeCampaign":      func() error { return s.NudgeCampaign(context.Background(), "", map[string]any{}) },
		"CampaignPhoneNumbers": func() error {
			_, err := s.CampaignPhoneNumbers(context.Background(), "", 10, 0)
			return err
		},
		"CampaignHistory": func() error {
			_, err := s.CampaignHistory(context.Background(), "", 10, 0)
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			got = captured{}
			if err := call(); err == nil {
				t.Fatal("want an error for an empty ID, got nil")
			}
			if got.method != "" {
				t.Errorf("a request was made (%s %s); want none", got.method, got.path)
			}
		})
	}
}

// IDs go into the path, so a value containing a slash or a space must be
// escaped rather than silently changing which endpoint is called. Assert on
// got.escapedPath, not got.path: net/url decodes Path, so an assertion
// against it would pass whether or not url.PathEscape was called.
func TestCampaignIDIsPathEscaped(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":[],"page":{"totalElements":0}}`, &got)

	if _, err := s.CampaignHistory(context.Background(), "a/b c", 10, 0); err != nil {
		t.Fatalf("CampaignHistory: %v", err)
	}
	if want := "/api/v2/accounts/9901287/tendlc/campaigns/a%2Fb%20c/history"; got.escapedPath != want {
		t.Errorf("escaped path = %q, want %q", got.escapedPath, want)
	}
}
