package tendlc

import (
	"fmt"
	"net/url"

	"github.com/Bandwidth/cli/internal/api"
)

// CreateCampaign submits a campaign for registration (direct customers) or
// refreshes an existing campaign from TCR (all customers, body =
// {"campaignId": id}). Both are POST /campaigns; which one happens is
// decided by the body, not the path.
func (s *Service) CreateCampaign(body map[string]any) (*api.Envelope, error) {
	raw, err := s.client.PostRaw(s.base()+"/campaigns", body)
	if err != nil {
		return nil, err
	}
	return api.ParseEnvelope(raw)
}

// UpdateCampaign replaces a campaign.
//
// The API treats PUT as a FULL REPLACEMENT — but only for direct campaigns.
// On an imported campaign, measured against production, a PUT with an empty
// body returned 202 and left campaignName, description, and sample1
// unchanged: it is not a replacement there. Campaigns carry no version
// field, so there is no optimistic-locking check either way: a concurrent
// edit between the GET and this PUT is lost silently.
//
// The PUT goes through putReplaceWithReadOnlyRetry, which retries once, with
// the named fields stripped, if the API rejects a 400 on read-only keys
// campaignReadOnlyFields already tried to remove — see that function for why.
func (s *Service) UpdateCampaign(campaignID string, body map[string]any) (*api.Envelope, error) {
	if campaignID == "" {
		return nil, fmt.Errorf("campaign ID is required")
	}
	raw, err := putReplaceWithReadOnlyRetry(s.client, s.campaignPath(campaignID), body)
	if err != nil {
		return nil, err
	}
	return api.ParseEnvelope(raw)
}

// DeactivateCampaign permanently deactivates a campaign. Returns 204.
func (s *Service) DeactivateCampaign(campaignID string) error {
	if campaignID == "" {
		return fmt.Errorf("campaign ID is required")
	}
	return s.client.Delete(s.campaignPath(campaignID), nil)
}

// NudgeCampaign resubmits a campaign stuck in a pending state to TCR for
// re-evaluation. Returns 204.
func (s *Service) NudgeCampaign(campaignID string, body map[string]any) error {
	if campaignID == "" {
		return fmt.Errorf("campaign ID is required")
	}
	return s.client.Post(s.campaignPath(campaignID)+"/nudge", body, nil)
}

// CampaignPhoneNumbers returns the phone numbers assigned to a campaign.
func (s *Service) CampaignPhoneNumbers(campaignID string, limit, offset int) (*api.Envelope, error) {
	if campaignID == "" {
		return nil, fmt.Errorf("campaign ID is required")
	}
	return s.get(s.campaignPath(campaignID) + "/phoneNumbers" + api.EncodeQuery(limit, offset, nil))
}

// CampaignHistory returns the campaign's activity log: free-text
// {createdDate, message} entries, newest first. As with BrandHistory there
// are no versioned snapshots and no per-version fetch.
func (s *Service) CampaignHistory(campaignID string, limit, offset int) (*api.Envelope, error) {
	if campaignID == "" {
		return nil, fmt.Errorf("campaign ID is required")
	}
	return s.get(s.campaignPath(campaignID) + "/history" + api.EncodeQuery(limit, offset, nil))
}

// campaignPath builds /campaigns/{id}.
func (s *Service) campaignPath(campaignID string) string {
	return s.base() + "/campaigns/" + url.PathEscape(campaignID)
}
