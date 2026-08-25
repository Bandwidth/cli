package tendlc

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Bandwidth/cli/internal/api"
)

// ListPhoneNumbers returns the phone numbers on the account.
//
// The projection is CONDITIONAL, not fixed. Measured across all 23 numbers
// on one account: 16 carried five keys — createdDate, modifiedDate, nnid,
// phoneNumber, status — while the 7 assigned to a campaign carried three
// more: brandId, campaignId, customerProfileId. A client that types this
// response from a single sample drops three fields on every assigned
// number. (An earlier version of this comment claimed a fixed five-key
// projection; that was measured from one record fetched with limit=1.)
//
// Filter support is split, and both halves were measured:
//
//   - campaignId[contains] WORKS. A campaign with three assigned numbers
//     returns 3; a garbage value returns 0. campaignId[eq] is ignored and
//     returns all 23, matching the brand endpoints' behavior in general.
//   - status does NOT filter under any operator. An account with 21 SUCCESS
//     and 2 FAILURE returned all 23 for status[eq] and status[contains] on
//     SUCCESS, on FAILURE, and on a value matching nothing at all.
//
// So the command layer offers --campaign-id-contains and deliberately does
// not offer --status.
func (s *Service) ListPhoneNumbers(ctx context.Context, limit, offset int, filters []api.Filter) (*api.Envelope, error) {
	return s.get(ctx, s.base()+"/phoneNumbers"+api.EncodeQuery(limit, offset, filters))
}

// GetPhoneNumber returns one phone number.
//
// Measured against production: this endpoint returned 404 for all four
// numbers tested, while PhoneNumberHistory on the same path prefix returned
// 200 for all four. The cause is unconfirmed — only one account was
// available to test against, and this API reports authorization failures as
// 403, so a 404 here is not a permissions mask in disguise. `band tendlc
// number get <tn>`, which calls this, inherits the same 404.
func (s *Service) GetPhoneNumber(ctx context.Context, phoneNumber string) (*api.Envelope, error) {
	if phoneNumber == "" {
		return nil, fmt.Errorf("phone number is required")
	}
	return s.get(ctx, s.phoneNumberPath(phoneNumber))
}

// PhoneNumberHistory returns the phone number's activity log: free-text
// {createdDate, message} entries, newest first. As with BrandHistory and
// CampaignHistory there are no versioned snapshots and no per-version fetch.
func (s *Service) PhoneNumberHistory(ctx context.Context, phoneNumber string, limit, offset int) (*api.Envelope, error) {
	if phoneNumber == "" {
		return nil, fmt.Errorf("phone number is required")
	}
	return s.get(ctx, s.phoneNumberPath(phoneNumber)+"/history"+api.EncodeQuery(limit, offset, nil))
}

// phoneNumberPath builds /phoneNumbers/{tn}.
//
// Phone numbers are E.164 and carry a leading '+'. url.PathEscape leaves '+'
// unescaped — verified directly: url.PathEscape("+15555550100") returns
// "+15555550100", not "%2B15555550100". That is correct: '+' is a valid
// sub-delimiter in a path segment, so this is not a bug to "fix" by
// switching to url.QueryEscape, which would encode it as %2B where it does
// not belong. Measured against production, a raw '+' and a pre-encoded %2B
// reach the server identically. PathEscape is called anyway, for
// consistency with brandPath/campaignPath and because it still needs to
// escape whatever isn't a '+'.
func (s *Service) phoneNumberPath(phoneNumber string) string {
	return s.base() + "/phoneNumbers/" + url.PathEscape(phoneNumber)
}
