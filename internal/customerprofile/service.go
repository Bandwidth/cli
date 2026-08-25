// Package customerprofile wraps Bandwidth's Numbers v2 customer-profile API.
//
// Customer profiles are a prerequisite for 10DLC brand registration, and a
// profile backs EXACTLY ONE brand — reusing one fails with "cannot be
// assigned to another brand". This 1:1 constraint is undocumented in the
// published specs and was found by probing production.
package customerprofile

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Bandwidth/cli/internal/api"
)

// Service wraps the customer-profile endpoints for one account.
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
	return "/api/v2/accounts/" + url.PathEscape(s.accountID) + "/customerProfiles"
}

func (s *Service) get(ctx context.Context, path string) (*api.Envelope, error) {
	raw, err := s.client.GetRaw(ctx, path)
	if err != nil {
		return nil, err
	}
	return api.ParseEnvelope(raw)
}

// List returns customer profiles on the account.
func (s *Service) List(ctx context.Context, limit, offset int, filters []api.Filter) (*api.Envelope, error) {
	return s.get(ctx, s.base()+api.EncodeQuery(limit, offset, filters))
}

// Get returns one customer profile. Soft-deleted profiles are still
// returned individually, with softDeleted set to true — check it before
// creating any association. There is no "deleted" field on reads.
func (s *Service) Get(ctx context.Context, profileID string) (*api.Envelope, error) {
	if profileID == "" {
		return nil, fmt.Errorf("customer profile ID is required")
	}
	return s.get(ctx, s.base()+"/"+url.PathEscape(profileID))
}
