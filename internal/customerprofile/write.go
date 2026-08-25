package customerprofile

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Bandwidth/cli/internal/api"
)

// Create posts a new customer profile. Callers build body via BuildCreateRequest.
func (s *Service) Create(ctx context.Context, body map[string]any) (*api.Envelope, error) {
	raw, err := s.client.PostRaw(ctx, s.base(), body)
	if err != nil {
		return nil, err
	}
	return api.ParseEnvelope(raw)
}

// Update replaces a customer profile.
//
// The API treats PUT as a FULL REPLACEMENT — a field omitted from body is set
// to null server-side — and requires the current version even though the
// schema marks version readOnly. Neither behavior is documented; both were
// measured against production. Callers must build body with
// BuildUpdateRequest, which starts from the current resource so nothing is
// dropped.
func (s *Service) Update(ctx context.Context, profileID string, body map[string]any) (*api.Envelope, error) {
	if profileID == "" {
		return nil, fmt.Errorf("customer profile ID is required")
	}
	raw, err := s.client.PutRawJSON(ctx, s.base()+"/"+url.PathEscape(profileID), body)
	if err != nil {
		return nil, err
	}
	return api.ParseEnvelope(raw)
}

// Delete soft-deletes a customer profile. The record remains retrievable by ID
// with softDeleted set to true, and can be restored — see BuildRestoreRequest.
func (s *Service) Delete(ctx context.Context, profileID string) error {
	if profileID == "" {
		return fmt.Errorf("customer profile ID is required")
	}
	return s.client.Delete(ctx, s.base()+"/"+url.PathEscape(profileID), nil)
}

// History returns the version history of a profile.
func (s *Service) History(ctx context.Context, profileID string, limit, offset int) (*api.Envelope, error) {
	if profileID == "" {
		return nil, fmt.Errorf("customer profile ID is required")
	}
	return s.get(ctx, s.base()+"/"+url.PathEscape(profileID)+"/history"+
		api.EncodeQuery(limit, offset, nil))
}

// HistoryVersion returns one historical version of a profile.
func (s *Service) HistoryVersion(ctx context.Context, profileID, version string) (*api.Envelope, error) {
	if profileID == "" {
		return nil, fmt.Errorf("customer profile ID is required")
	}
	if version == "" {
		return nil, fmt.Errorf("version is required")
	}
	return s.get(ctx, s.base()+"/"+url.PathEscape(profileID)+
		"/history/"+url.PathEscape(version))
}
