package tendlc

import (
	"fmt"
	"net/url"

	"github.com/Bandwidth/cli/internal/api"
)

// ListPhoneNumbers returns the phone numbers on the account. The list
// projection has five keys — createdDate, modifiedDate, nnid, phoneNumber,
// status — notably no campaignId.
//
// filters is accepted only for signature consistency with ListBrands and
// ListCampaigns. Measured against production, filtering does not work on
// this endpoint: status[eq] is silently ignored (an account with 21 SUCCESS
// and 2 FAILURE phone numbers returned all 23 for status[eq]=SUCCESS,
// status[eq]=FAILED, and status[eq]=NOT_A_STATUS alike), and
// campaignId[contains] is evaluated but matches nothing, for a real
// campaign ID and for garbage alike. No caller should pass a filter here,
// and the command layer offers no flags for one.
func (s *Service) ListPhoneNumbers(limit, offset int, filters []api.Filter) (*api.Envelope, error) {
	return s.get(s.base() + "/phoneNumbers" + api.EncodeQuery(limit, offset, filters))
}

// GetPhoneNumber returns one phone number.
//
// Measured against production: this endpoint returned 404 for all four
// numbers tested, while PhoneNumberHistory on the same path prefix returned
// 200 for all four. The cause is unconfirmed — only one account was
// available to test against, and this API reports authorization failures as
// 403, so a 404 here is not a permissions mask in disguise. The currently
// shipped `band tendlc number <tn>` command already fails the same way.
func (s *Service) GetPhoneNumber(phoneNumber string) (*api.Envelope, error) {
	if phoneNumber == "" {
		return nil, fmt.Errorf("phone number is required")
	}
	return s.get(s.phoneNumberPath(phoneNumber))
}

// PhoneNumberHistory returns the phone number's activity log: free-text
// {createdDate, message} entries, newest first. As with BrandHistory and
// CampaignHistory there are no versioned snapshots and no per-version fetch.
func (s *Service) PhoneNumberHistory(phoneNumber string, limit, offset int) (*api.Envelope, error) {
	if phoneNumber == "" {
		return nil, fmt.Errorf("phone number is required")
	}
	return s.get(s.phoneNumberPath(phoneNumber) + "/history" + api.EncodeQuery(limit, offset, nil))
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
