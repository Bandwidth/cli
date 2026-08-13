// Package tendlc wraps Bandwidth's v2 A2P Campaign Management (10DLC
// Registration Center) API.
package tendlc

import (
	"fmt"
	"net/url"

	"github.com/Bandwidth/cli/internal/api"
)

// Service wraps the v2 A2P Campaign Management endpoints for one account.
type Service struct {
	client    *api.Client
	accountID string
}

// NewService returns a Service bound to an account. c must be a JSON client
// (see cmdutil.PlatformClient).
func NewService(c *api.Client, accountID string) *Service {
	return &Service{client: c, accountID: accountID}
}

func (s *Service) base() string {
	return "/api/v2/accounts/" + url.PathEscape(s.accountID) + "/tendlc"
}

// get issues a GET and parses the standard {data, page} envelope.
func (s *Service) get(path string) (*api.Envelope, error) {
	raw, err := s.client.GetRaw(path)
	if err != nil {
		return nil, err
	}
	return api.ParseEnvelope(raw)
}

// ListBrands returns the brands on the account.
func (s *Service) ListBrands(limit, offset int, filters []api.Filter) (*api.Envelope, error) {
	return s.get(s.base() + "/brands" + api.EncodeQuery(limit, offset, filters))
}

// GetBrand returns one brand. The response wraps data as an object, not a
// single-element array — verified against production.
func (s *Service) GetBrand(brandID string) (*api.Envelope, error) {
	if brandID == "" {
		return nil, fmt.Errorf("brand ID is required")
	}
	return s.get(s.base() + "/brands/" + url.PathEscape(brandID))
}

// ListCampaigns returns the campaigns on the account. The list projection
// omits fields the campaign schema defines (imported, cspId, samples,
// messageFlow) — use GetCampaign when those are needed.
func (s *Service) ListCampaigns(limit, offset int, filters []api.Filter) (*api.Envelope, error) {
	return s.get(s.base() + "/campaigns" + api.EncodeQuery(limit, offset, filters))
}

// GetCampaign returns one campaign, including the fields the list omits.
func (s *Service) GetCampaign(campaignID string) (*api.Envelope, error) {
	if campaignID == "" {
		return nil, fmt.Errorf("campaign ID is required")
	}
	return s.get(s.base() + "/campaigns/" + url.PathEscape(campaignID))
}
