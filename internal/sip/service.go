package sip

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/output"
)

// APIFault is a structured Bandwidth error (ErrorCode + Description). Commands
// branch on Code to map documented failures onto exit codes.
type APIFault struct {
	Code        string
	Description string
	StatusCode  int
}

func (e *APIFault) Error() string {
	return fmt.Sprintf("%s (error %s)", e.Description, e.Code)
}

// Service wraps the Dashboard SIP endpoints for one account.
type Service struct {
	client    *api.Client
	accountID string
}

// NewService returns a Service bound to an account. c must be an XML client
// (see cmdutil.DashboardClient).
func NewService(c *api.Client, accountID string) *Service {
	return &Service{client: c, accountID: accountID}
}

func (s *Service) base() string {
	return "/accounts/" + url.PathEscape(s.accountID)
}

// do issues a request and returns the response body, converting documented
// error envelopes into *APIFault. Bodies are scrubbed of digest hashes before
// ever being placed in an error.
func (s *Service) do(method, path string, reqBody interface{}) ([]byte, error) {
	var payload []byte
	if reqBody != nil {
		b, err := xml.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("encoding request: %w", err)
		}
		payload = append([]byte(xml.Header), b...)
	}

	resp, err := s.client.DoRawResponse(method, path, payload)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.Body, nil
	}
	if fault := parseFault(resp.Body, resp.StatusCode); fault != nil {
		return nil, fault
	}
	return nil, &api.APIError{StatusCode: resp.StatusCode, Body: output.ScrubHashes(string(resp.Body))}
}

// parseFault extracts ResponseStatus or the first Errors entry. Returns nil if
// the body carries neither.
func parseFault(body []byte, status int) *APIFault {
	var probe struct {
		ResponseStatus *responseStatus `xml:"ResponseStatus"`
		Errors         []wireError     `xml:"Errors>Error"`
	}
	if err := xml.Unmarshal(body, &probe); err != nil {
		// Unparseable body: discard it entirely rather than risk leaking hashes.
		return &APIFault{Code: "", Description: "unreadable error response", StatusCode: status}
	}
	if probe.ResponseStatus != nil && probe.ResponseStatus.ErrorCode != "" {
		return &APIFault{
			Code:        probe.ResponseStatus.ErrorCode,
			Description: probe.ResponseStatus.Description,
			StatusCode:  status,
		}
	}
	if len(probe.Errors) > 0 {
		return &APIFault{
			Code:        probe.Errors[0].ErrorCode,
			Description: probe.Errors[0].Description,
			StatusCode:  status,
		}
	}
	return nil
}

// shortName derives the realm's short name from its FQDN. The API returns the
// FQDN in the same element used to submit the short name.
func shortName(fqdn string) string {
	if i := strings.Index(fqdn, "."); i >= 0 {
		label := fqdn[:i]
		if j := strings.LastIndex(label, "-"); j >= 0 {
			return label[:j] // strip the "-<accountHex>" suffix
		}
		return label
	}
	return fqdn
}

func toRealm(w *realmWire) *Realm {
	return &Realm{
		ID:              w.ID,
		Name:            shortName(w.Realm),
		Hostname:        w.Realm,
		Description:     w.Description,
		Default:         w.Default,
		Status:          w.Status,
		CredentialCount: w.SipCredentialCount,
	}
}

func toCredential(w *credentialWire) *Credential {
	return &Credential{
		ID:       w.ID,
		RealmID:  w.RealmID,
		Username: w.UserName,
		Hostname: w.Realm,
		AppID:    w.AppID,
	}
}

// CreateRealm creates a realm. isDefault is always transmitted: the API rejects
// the request without it (error 1003).
func (s *Service) CreateRealm(name, description string, isDefault bool) (*Realm, error) {
	body, err := s.do("POST", s.base()+"/realms", realmRequest{
		Realm: name, Description: description, Default: isDefault,
	})
	if err != nil {
		return nil, err
	}
	var resp realmResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding realm response: %w", err)
	}
	if resp.Realm == nil {
		return nil, fmt.Errorf("realm response contained no realm")
	}
	return toRealm(resp.Realm), nil
}

// GetRealm fetches one realm. ref may be an ID or a name.
func (s *Service) GetRealm(ref string) (*Realm, error) {
	body, err := s.do("GET", s.base()+"/realms/"+url.PathEscape(ref), nil)
	if err != nil {
		return nil, err
	}
	var resp realmResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding realm response: %w", err)
	}
	if resp.Realm == nil {
		return nil, fmt.Errorf("realm %q not found", ref)
	}
	return toRealm(resp.Realm), nil
}

// ListRealms returns every realm on the account, always as a non-nil slice.
func (s *Service) ListRealms() ([]Realm, error) {
	body, err := s.do("GET", s.base()+"/realms", nil)
	if err != nil {
		return nil, err
	}
	var resp realmsResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding realms response: %w", err)
	}
	out := make([]Realm, 0, len(resp.Realms))
	for i := range resp.Realms {
		out = append(out, *toRealm(&resp.Realms[i]))
	}
	return out, nil
}

// DeleteRealm submits an async delete (the API returns 202).
func (s *Service) DeleteRealm(ref string) error {
	_, err := s.do("DELETE", s.base()+"/realms/"+url.PathEscape(ref), nil)
	return err
}

func (s *Service) credentialsPath(realmID string) string {
	return s.base() + "/realms/" + url.PathEscape(realmID) + "/sipcredentials"
}

// CreateCredential creates one credential. A 201 carrying an Errors entry is
// treated as failure, not success.
func (s *Service) CreateCredential(realmID, username, hash1, hash1b, appID string) (*Credential, error) {
	req := credentialCreateRequest{Credentials: []credentialCreateOne{{
		UserName: username, Hash1: hash1, Hash1b: hash1b, AppID: appID,
	}}}
	body, err := s.do("POST", s.credentialsPath(realmID), req)
	if err != nil {
		return nil, err
	}
	var resp credentialsResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding credential response: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, &APIFault{Code: resp.Errors[0].ErrorCode, Description: resp.Errors[0].Description, StatusCode: 201}
	}
	for i := range resp.Valid {
		if resp.Valid[i].UserName == username {
			return toCredential(&resp.Valid[i]), nil
		}
	}
	return nil, fmt.Errorf("credential %q was not returned in ValidSipCredentials", username)
}

// RotateCredential replaces a credential's hashes. The credential ID is stable
// across rotation. UserName must not be sent (see credentialRotateRequest).
func (s *Service) RotateCredential(realmID, credentialID, hash1, hash1b string) (*Credential, error) {
	req := credentialRotateRequest{RealmID: realmID, Hash1: hash1, Hash1b: hash1b}
	body, err := s.do("PUT", s.credentialsPath(realmID)+"/"+url.PathEscape(credentialID), req)
	if err != nil {
		return nil, err
	}
	var resp credentialResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding credential response: %w", err)
	}
	if resp.Credential == nil {
		return nil, fmt.Errorf("rotate response contained no credential")
	}
	return toCredential(resp.Credential), nil
}

// ListCredentials returns a realm's credentials, always as a non-nil slice.
// The API answers an unpaginated request with a 303 to ?page=1&size=500, which
// the client follows.
func (s *Service) ListCredentials(realmID string) ([]Credential, error) {
	body, err := s.do("GET", s.credentialsPath(realmID), nil)
	if err != nil {
		return nil, err
	}
	var resp credentialsResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding credentials response: %w", err)
	}
	out := make([]Credential, 0, len(resp.Credentials))
	for i := range resp.Credentials {
		out = append(out, *toCredential(&resp.Credentials[i]))
	}
	return out, nil
}

// GetCredential fetches one credential.
func (s *Service) GetCredential(realmID, credentialID string) (*Credential, error) {
	body, err := s.do("GET", s.credentialsPath(realmID)+"/"+url.PathEscape(credentialID), nil)
	if err != nil {
		return nil, err
	}
	var resp credentialResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding credential response: %w", err)
	}
	if resp.Credential == nil {
		return nil, fmt.Errorf("credential %q not found", credentialID)
	}
	return toCredential(resp.Credential), nil
}

// DeleteCredential removes a credential.
func (s *Service) DeleteCredential(realmID, credentialID string) error {
	_, err := s.do("DELETE", s.credentialsPath(realmID)+"/"+url.PathEscape(credentialID), nil)
	return err
}
