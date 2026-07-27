package sip

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"time"

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

// Unwrap lets errors.As match *api.APIError, so cmdutil.ExitCodeForError maps
// SIP faults onto the CLI's exit-code taxonomy by StatusCode (401/403->2,
// 404->3, 409->4, 429->7) instead of every documented failure exiting 1.
func (e *APIFault) Unwrap() error {
	return &api.APIError{StatusCode: e.StatusCode, Body: e.Description}
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
		// An empty body (e.g. a 202 delete) is unambiguous success: there is
		// nothing to unmarshal and nothing to fault.
		if len(resp.Body) == 0 {
			return resp.Body, nil
		}
		// A 2xx can still carry an error envelope (e.g. a partially successful
		// bulk credential create), so probe it the same as a non-2xx body.
		if fault := parseFault(resp.Body, resp.StatusCode); fault != nil {
			return nil, fault
		}
		return resp.Body, nil
	}
	if fault := parseFault(resp.Body, resp.StatusCode); fault != nil {
		return nil, fault
	}
	return nil, &api.APIError{StatusCode: resp.StatusCode, Body: output.ScrubHashes(string(resp.Body))}
}

// parseFault extracts ResponseStatus or the first Errors entry. Returns nil if
// the body carries neither, including when the body is not parseable XML — the
// caller falls through to the api.APIError path, which already scrubs hashes
// and substitutes a status-text placeholder when the body is empty.
func parseFault(body []byte, status int) *APIFault {
	var probe struct {
		ResponseStatus *responseStatus `xml:"ResponseStatus"`
		Errors         []wireError     `xml:"Errors>Error"`
	}
	if err := xml.Unmarshal(body, &probe); err != nil {
		return nil
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

// SetRealmDefault promotes a realm to the account default. The API only
// supports setting Default to true.
func (s *Service) SetRealmDefault(ref string) (*Realm, error) {
	current, err := s.GetRealm(ref)
	if err != nil {
		return nil, err
	}
	// Read-modify-write: Description is resent so a full-replace PUT cannot drop it.
	body, err := s.do("PUT", s.base()+"/realms/"+url.PathEscape(ref), realmRequest{
		Realm: current.Name, Description: current.Description, Default: true,
	})
	if err != nil {
		return nil, err
	}
	var resp realmResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding realm response: %w", err)
	}
	if resp.Realm == nil {
		return s.GetRealm(ref)
	}
	return toRealm(resp.Realm), nil
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
	// A case-insensitive match means the API echoed a different case than what
	// ComputeHashes used to build the digest: the credential exists server-side
	// but its hashes are for the wrong username and it can never authenticate.
	for i := range resp.Valid {
		if strings.EqualFold(resp.Valid[i].UserName, username) {
			return nil, fmt.Errorf(
				"API returned username %q, expected %q; the credential exists but its digest hashes are invalid — delete it and retry",
				resp.Valid[i].UserName, username)
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

// FindCredentialByUsername returns the single credential with this username in
// a realm. Bounded retry absorbs read-after-write lag following a duplicate error.
func (s *Service) FindCredentialByUsername(realmID, username string) (*Credential, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Second)
		}
		creds, err := s.ListCredentials(realmID)
		if err != nil {
			lastErr = err
			continue
		}
		var matches []Credential
		for i := range creds {
			if creds[i].Username == username {
				matches = append(matches, creds[i])
			}
		}
		switch len(matches) {
		case 1:
			return &matches[0], nil
		case 0:
			lastErr = fmt.Errorf("credential %q not found in realm %s", username, realmID)
		default:
			return nil, fmt.Errorf("found %d credentials named %q in realm %s; delete the duplicates", len(matches), username, realmID)
		}
	}
	return nil, lastErr
}

// credentialHashWire exists only for equality checks. The public credentialWire
// deliberately carries no hash fields so digest material cannot reach output;
// this private type keeps the comparison inside the service.
type credentialHashWire struct {
	Credential struct {
		Hash1  string `xml:"Hash1"`
		Hash1b string `xml:"Hash1b"`
	} `xml:"SipCredential"`
}

// CredentialHashesMatch reports whether the stored digest hashes equal the
// supplied ones. The hashes never leave this function.
func (s *Service) CredentialHashesMatch(realmID, credentialID, hash1, hash1b string) (bool, error) {
	body, err := s.do("GET", s.credentialsPath(realmID)+"/"+url.PathEscape(credentialID), nil)
	if err != nil {
		return false, err
	}
	var w credentialHashWire
	if err := xml.Unmarshal(body, &w); err != nil {
		return false, fmt.Errorf("decoding credential response: %w", err)
	}
	return w.Credential.Hash1 == hash1 && w.Credential.Hash1b == hash1b, nil
}
