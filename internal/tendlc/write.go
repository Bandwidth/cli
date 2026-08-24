package tendlc

import (
	"fmt"
	"net/url"

	"github.com/Bandwidth/cli/internal/api"
)

// CreateBrand submits a brand for registration (direct customers) or refreshes
// an existing brand from TCR (all customers, body = {"brandId": id}). Both are
// POST /brands; which one happens is decided by the body, not the path.
// Returns 202 with a bandwidthId — the TCR brandId may not exist yet.
func (s *Service) CreateBrand(body map[string]any) (*api.Envelope, error) {
	raw, err := s.client.PostRaw(s.base()+"/brands", body)
	if err != nil {
		return nil, err
	}
	return api.ParseEnvelope(raw)
}

// UpdateBrand replaces a brand.
//
// The API treats PUT as a FULL REPLACEMENT — a field omitted from body is set
// to null server-side. Callers must build body with BuildBrandUpdateRequest,
// which starts from the current resource so nothing is dropped. Brands carry
// no version field, so there is no optimistic-locking check: a concurrent
// edit between the GET and this PUT is lost silently.
//
// The PUT goes through putReplaceWithReadOnlyRetry, which retries once, with
// the named fields stripped, if the API rejects a 400 on read-only keys
// brandReadOnlyFields already tried to remove — see that function for why.
// The retry's neverDrop set is brandNeverDropFields(): every JSON key any
// `brand update` flag can write, not merely the ones changed THIS call — a
// 400 naming a field reachable by any update flag (e.g. website, even when
// this call never touched it) surfaces as a real validation failure instead
// of being silently stripped and re-sent. See putReplaceWithReadOnlyRetry's
// INVARIANT for why "changed this call" is not the right test.
func (s *Service) UpdateBrand(brandID string, body map[string]any) (*api.Envelope, error) {
	if brandID == "" {
		return nil, fmt.Errorf("brand ID is required")
	}
	raw, err := putReplaceWithReadOnlyRetry(s.client, s.brandPath(brandID), body, brandNeverDropFields())
	if err != nil {
		return nil, err
	}
	return api.ParseEnvelope(raw)
}

// DeleteBrand permanently deletes a brand. For direct accounts this also
// deletes the brand in TCR, and all campaigns must be deactivated first. It
// does NOT cascade to the associated customer profile — the endpoint docs
// claim it does, but measured against production, both test profiles
// survived with softDeleted: false. Delete the profile separately if it is
// no longer needed.
func (s *Service) DeleteBrand(brandID string) error {
	if brandID == "" {
		return fmt.Errorf("brand ID is required")
	}
	return s.client.Delete(s.brandPath(brandID), nil)
}

// ReverifyBrand resubmits the brand for identity verification. This incurs a
// $4 fee and resets brandIdentityStatus toward re-registration — documented
// as REGISTERING, but production reads it back as UNVERIFIED until TCR
// responds. Returns 204.
func (s *Service) ReverifyBrand(brandID string) error {
	if brandID == "" {
		return fmt.Errorf("brand ID is required")
	}
	return s.client.Post(s.brandPath(brandID)+"/identity/reverify", nil, nil)
}

// Resend2FA re-sends the Business Authentication 2FA email to the brand's
// business contact. Returns 204.
func (s *Service) Resend2FA(brandID string) error {
	if brandID == "" {
		return fmt.Errorf("brand ID is required")
	}
	return s.client.Post(s.brandPath(brandID)+"/identity/resend2faEmail", nil, nil)
}

// BrandHistory returns the brand's activity log: free-text {createdDate,
// message} entries, newest first. Unlike customer profiles there are no
// versioned snapshots and no per-version fetch.
func (s *Service) BrandHistory(brandID string, limit, offset int) (*api.Envelope, error) {
	if brandID == "" {
		return nil, fmt.Errorf("brand ID is required")
	}
	return s.get(s.brandPath(brandID) + "/history" + api.EncodeQuery(limit, offset, nil))
}

// ListVettings returns the external vettings on a brand. Vettings are
// brand-scoped: there is no campaign vetting endpoint.
func (s *Service) ListVettings(brandID string, limit, offset int) (*api.Envelope, error) {
	if brandID == "" {
		return nil, fmt.Errorf("brand ID is required")
	}
	return s.get(s.brandPath(brandID) + "/vettings" + api.EncodeQuery(limit, offset, nil))
}

// RequestVetting orders a new external vetting for a brand. Billable.
func (s *Service) RequestVetting(brandID string, body map[string]any) (*api.Envelope, error) {
	if brandID == "" {
		return nil, fmt.Errorf("brand ID is required")
	}
	raw, err := s.client.PostRaw(s.brandPath(brandID)+"/vettings", body)
	if err != nil {
		return nil, err
	}
	return api.ParseEnvelope(raw)
}

// ImportVetting records an externally-performed vetting against a brand.
func (s *Service) ImportVetting(brandID, vettingID string, body map[string]any) (*api.Envelope, error) {
	if brandID == "" {
		return nil, fmt.Errorf("brand ID is required")
	}
	if vettingID == "" {
		return nil, fmt.Errorf("vetting ID is required")
	}
	raw, err := s.client.PutRawJSON(
		s.brandPath(brandID)+"/vettings/"+url.PathEscape(vettingID), body)
	if err != nil {
		return nil, err
	}
	return api.ParseEnvelope(raw)
}

// brandPath builds /brands/{id}. The path parameter is documented as the TCR
// brandId, but production accepts the Bandwidth ID too — which is required,
// not merely convenient: a newly created direct brand has brandId null until
// TCR registers it, and would otherwise be unreachable.
func (s *Service) brandPath(brandID string) string {
	return s.base() + "/brands/" + url.PathEscape(brandID)
}
