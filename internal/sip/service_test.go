package sip

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
)

func newTestService(t *testing.T, h http.HandlerFunc) (*Service, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	c := api.NewXMLClient(srv.URL, nil)
	return NewService(c, "9901361"), srv.Close
}

func TestCreateRealm_ParsesFQDNAndStatus(t *testing.T) {
	var gotBody string
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(201)
		w.Write([]byte(`<?xml version='1.0'?><RealmResponse><Realm>` +
			`<Id>1103</Id><Realm>bwclitest-3efeaa.auth.bandwidth.com</Realm>` +
			`<Description>d</Description><Default>false</Default>` +
			`<SipCredentialCount>0</SipCredentialCount><Status>CREATE_PENDING</Status>` +
			`</Realm></RealmResponse>`))
	})
	defer done()

	r, err := svc.CreateRealm("bwclitest", "d", false)
	if err != nil {
		t.Fatalf("CreateRealm() error = %v", err)
	}
	if r.ID != "1103" {
		t.Errorf("ID = %q, want 1103", r.ID)
	}
	// The API returns the FQDN in <Realm>; the short name is derived, never built.
	if r.Hostname != "bwclitest-3efeaa.auth.bandwidth.com" {
		t.Errorf("Hostname = %q", r.Hostname)
	}
	if r.Name != "bwclitest" {
		t.Errorf("Name = %q, want bwclitest", r.Name)
	}
	if r.Status != "CREATE_PENDING" {
		t.Errorf("Status = %q, want CREATE_PENDING", r.Status)
	}
	if r.Default {
		t.Error("Default = true, want false")
	}
	// Default is required by the API; it must always be sent.
	if !strings.Contains(gotBody, "<Default>false</Default>") {
		t.Errorf("request body missing Default element: %s", gotBody)
	}
}

func TestCreateRealm_ReturnsAPIFault(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`<RealmResponse><ResponseStatus><ErrorCode>33004</ErrorCode>` +
			`<Description>Your account isn't setup for Sip Credentials.</Description>` +
			`</ResponseStatus></RealmResponse>`))
	})
	defer done()

	_, err := svc.CreateRealm("x", "", false)
	var fault *APIFault
	if !errorsAs(err, &fault) {
		t.Fatalf("error = %v (%T), want *APIFault", err, err)
	}
	if fault.Code != "33004" {
		t.Errorf("Code = %q, want 33004", fault.Code)
	}
}

func TestListCredentials_FollowsRedirectAndAlwaysReturnsSlice(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "" {
			http.Redirect(w, r, r.URL.Path+"?page=1&size=500", http.StatusSeeOther)
			return
		}
		w.Write([]byte(`<SipCredentialsResponse><SipCredentials/></SipCredentialsResponse>`))
	})
	defer done()

	creds, err := svc.ListCredentials("1103")
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if creds == nil {
		t.Fatal("ListCredentials() = nil, want empty non-nil slice")
	}
	if len(creds) != 0 {
		t.Errorf("len = %d, want 0", len(creds))
	}
}

func TestRotateCredential_SendsRealmIDAndOmitsUserName(t *testing.T) {
	// Live-verified: RealmId is required in the body even though it is in the
	// path (23031 without it), and sending UserName fails with 23030 even when
	// the value is unchanged.
	var gotBody string
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`<SipCredentialResponse><SipCredential>` +
			`<Id>870880</Id><RealmId>1105</RealmId><UserName>rotuser</UserName>` +
			`<Realm>rottest-3efeaa.auth.bandwidth.com</Realm>` +
			`</SipCredential></SipCredentialResponse>`))
	})
	defer done()

	c, err := svc.RotateCredential("1105", "870880", "h1", "h1b")
	if err != nil {
		t.Fatalf("RotateCredential() error = %v", err)
	}
	if !strings.Contains(gotBody, "<RealmId>1105</RealmId>") {
		t.Errorf("body missing RealmId: %s", gotBody)
	}
	if strings.Contains(gotBody, "UserName") {
		t.Errorf("body must not contain UserName: %s", gotBody)
	}
	if c.ID != "870880" {
		t.Errorf("ID = %q, want 870880 (must be stable across rotation)", c.ID)
	}
	if c.Username != "rotuser" {
		t.Errorf("Username = %q, want rotuser", c.Username)
	}
}

func TestCreateCredential_PartialSuccessIsFailure(t *testing.T) {
	// Live-verified: a 201 can still carry an Errors array; the requested
	// credential must be present in ValidSipCredentials or the command fails.
	// The status is 201 (not 400) so this actually exercises the 2xx+Errors
	// branch instead of the ordinary non-2xx fault path.
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		w.Write([]byte(`<SipCredentialsResponse><Errors><Error>` +
			`<ErrorCode>23026</ErrorCode><Description>does already exist</Description>` +
			`</Error></Errors></SipCredentialsResponse>`))
	})
	defer done()

	_, err := svc.CreateCredential("1103", "clitest", "h1", "h1b", "")
	var fault *APIFault
	if !errorsAs(err, &fault) || fault.Code != "23026" {
		t.Fatalf("error = %v, want APIFault 23026", err)
	}
}

func TestCreateCredential_NotInValidListIsFailure(t *testing.T) {
	// A 201 with ValidSipCredentials present, but missing the requested
	// username entirely, must fail rather than report success for a
	// credential that does not exist as requested.
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		w.Write([]byte(`<SipCredentialsResponse><ValidSipCredentials><SipCredential>` +
			`<Id>1</Id><UserName>someoneelse</UserName>` +
			`</SipCredential></ValidSipCredentials></SipCredentialsResponse>`))
	})
	defer done()

	_, err := svc.CreateCredential("1103", "clitest", "h1", "h1b", "")
	if err == nil {
		t.Fatal("CreateCredential() error = nil, want error")
	}
	var fault *APIFault
	if errorsAs(err, &fault) {
		t.Fatalf("error = %v, want plain error (no matching or case-folded username), not *APIFault", err)
	}
}

func TestCreateCredential_CaseMismatchReportsUnusableCredential(t *testing.T) {
	// The API echoed a different case than what was submitted. ComputeHashes
	// used the submitted case to build the digest, so the credential that
	// exists server-side can never authenticate — the error must say so
	// distinctly from "not returned at all".
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		w.Write([]byte(`<SipCredentialsResponse><ValidSipCredentials><SipCredential>` +
			`<Id>1</Id><UserName>CliTest</UserName>` +
			`</SipCredential></ValidSipCredentials></SipCredentialsResponse>`))
	})
	defer done()

	_, err := svc.CreateCredential("1103", "clitest", "h1", "h1b", "")
	if err == nil {
		t.Fatal("CreateCredential() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "digest hashes are invalid") {
		t.Errorf("error = %q, want mention of invalid digest hashes", err.Error())
	}
}

func TestDo_2xxEmptyBodyIsSuccess(t *testing.T) {
	// Live-verified: DELETE /realms/{id} returns 202 with a completely empty
	// body. That must not be misread as an unparseable/faulted response.
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
	})
	defer done()

	if err := svc.DeleteRealm("1103"); err != nil {
		t.Fatalf("DeleteRealm() error = %v, want nil for empty 202 body", err)
	}
}

func TestDo_2xxErrorEnvelopeIsFailure(t *testing.T) {
	// A 200 carrying a ResponseStatus/ErrorCode is not success, even though the
	// status code says so. This guards ListRealms and friends from reporting
	// "no realms" when the true state is "not authorized".
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`<RealmsResponse><ResponseStatus><ErrorCode>12666</ErrorCode>` +
			`<Description>not authorized</Description></ResponseStatus></RealmsResponse>`))
	})
	defer done()

	_, err := svc.ListRealms()
	var fault *APIFault
	if !errorsAs(err, &fault) || fault.Code != "12666" {
		t.Fatalf("error = %v, want APIFault 12666", err)
	}
}

func TestAPIFault_UnwrapsToAPIErrorForExitCodeMapping(t *testing.T) {
	// cmdutil.ExitCodeForError maps exit codes via errors.As(err, &apiErr) on
	// *api.APIError. Without Unwrap, every documented SIP failure (any fault
	// that carries an ErrorCode) would exit 1 regardless of HTTP status.
	fault := &APIFault{Code: "33004", Description: "conflict", StatusCode: 409}
	var apiErr *api.APIError
	if !errorsAs(fault, &apiErr) {
		t.Fatalf("errors.As(%v, &apiErr) = false, want true", fault)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("StatusCode = %d, want 409", apiErr.StatusCode)
	}
}

func TestParseFault_UnparseableBodyFallsThroughToAPIError(t *testing.T) {
	// A non-2xx body that isn't well-formed XML (or is empty) must not become
	// an APIFault with an empty Code — that renders as the useless
	// "(error )" and never carries a body for api.APIError to work with.
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("not xml at all"))
	})
	defer done()

	_, err := svc.GetRealm("1103")
	var fault *APIFault
	if errorsAs(err, &fault) {
		t.Fatalf("error = %v, want *api.APIError, not *APIFault", err)
	}
	var apiErr *api.APIError
	if !errorsAs(err, &apiErr) {
		t.Fatalf("error = %v (%T), want *api.APIError", err, err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", apiErr.StatusCode)
	}
}

func TestDo_ScrubsHashesFromUnfaultedErrorBody(t *testing.T) {
	// The single most security-relevant line in this file: output.ScrubHashes
	// must run before a raw error body (which can echo Hash1/Hash1b) is ever
	// placed on an error that gets printed to stderr.
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`<SomeResponse><Hash1>deadbeef</Hash1></SomeResponse>`))
	})
	defer done()

	_, err := svc.CreateRealm("x", "", false)
	if err == nil {
		t.Fatal("CreateRealm() error = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "[REDACTED]") {
		t.Errorf("error message missing [REDACTED]: %s", msg)
	}
	if strings.Contains(msg, "deadbeef") {
		t.Errorf("error message leaked hash value: %s", msg)
	}
}

func TestShortName(t *testing.T) {
	tests := []struct {
		name string
		fqdn string
		want string
	}{
		{"hyphenated label strips account suffix", "vapi-test-3efeaa.auth.bandwidth.com", "vapi-test"},
		{"no hyphen in label", "vapi3efeaa.auth.bandwidth.com", "vapi3efeaa"},
		{"empty string", "", ""},
		{"bare label with no dot", "vapi-test", "vapi-test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shortName(tt.fqdn); got != tt.want {
				t.Errorf("shortName(%q) = %q, want %q", tt.fqdn, got, tt.want)
			}
		})
	}
}

// errorsAs keeps the assertions above readable.
func errorsAs(err error, target interface{}) bool {
	return errors.As(err, target)
}

func TestFindCredentialByUsername_ZeroMatches(t *testing.T) {
	// The API reported a duplicate (that's the only caller of this method),
	// but the list genuinely doesn't contain the username after all retries.
	// The error must say so distinctly from "found it, multiple times".
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<SipCredentialsResponse><SipCredentials><SipCredential>` +
			`<Id>1</Id><UserName>someoneelse</UserName></SipCredential>` +
			`</SipCredentials></SipCredentialsResponse>`))
	})
	defer done()

	_, err := svc.FindCredentialByUsername("1103", "missing")
	if err == nil {
		t.Fatal("FindCredentialByUsername() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "no credential named") {
		t.Errorf("error = %q, want mention of the missing credential", err.Error())
	}
}

func TestFindCredentialByUsername_OneMatchIsCaseInsensitive(t *testing.T) {
	// The API's duplicate check (23026) is case-insensitive (see
	// CreateCredential's EqualFold branch); this lookup must match the same
	// way or a caller who requested "Agent" will never find the stored
	// "agent" and will spuriously report "not found" after a real duplicate.
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<SipCredentialsResponse><SipCredentials><SipCredential>` +
			`<Id>42</Id><UserName>Agent</UserName></SipCredential>` +
			`</SipCredentials></SipCredentialsResponse>`))
	})
	defer done()

	cred, err := svc.FindCredentialByUsername("1103", "agent")
	if err != nil {
		t.Fatalf("FindCredentialByUsername() error = %v", err)
	}
	if cred.ID != "42" {
		t.Errorf("ID = %q, want 42", cred.ID)
	}
}

func TestFindCredentialByUsername_MultipleMatchesIsError(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<SipCredentialsResponse><SipCredentials>` +
			`<SipCredential><Id>1</Id><UserName>agent</UserName></SipCredential>` +
			`<SipCredential><Id>2</Id><UserName>AGENT</UserName></SipCredential>` +
			`</SipCredentials></SipCredentialsResponse>`))
	})
	defer done()

	_, err := svc.FindCredentialByUsername("1103", "agent")
	if err == nil {
		t.Fatal("FindCredentialByUsername() error = nil, want error for multiple matches")
	}
	if !strings.Contains(err.Error(), "delete the duplicates") {
		t.Errorf("error = %q, want mention of duplicates", err.Error())
	}
}

func TestFindCredentialByUsername_TransportErrorThenSuccess(t *testing.T) {
	// A transient failure on one attempt must not abort the bounded retry —
	// only the final attempt's error (or a real 0/N+ match outcome) should
	// surface.
	var calls int
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(500)
			w.Write([]byte("boom"))
			return
		}
		w.Write([]byte(`<SipCredentialsResponse><SipCredentials><SipCredential>` +
			`<Id>7</Id><UserName>agent</UserName></SipCredential>` +
			`</SipCredentials></SipCredentialsResponse>`))
	})
	defer done()

	cred, err := svc.FindCredentialByUsername("1103", "agent")
	if err != nil {
		t.Fatalf("FindCredentialByUsername() error = %v", err)
	}
	if cred.ID != "7" {
		t.Errorf("ID = %q, want 7", cred.ID)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (one failure, then one success)", calls)
	}
}

func TestCredentialHashesMatch_Match(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<SipCredentialResponse><SipCredential>` +
			`<Id>870880</Id><Hash1>h1</Hash1><Hash1b>h1b</Hash1b>` +
			`</SipCredential></SipCredentialResponse>`))
	})
	defer done()

	match, err := svc.CredentialHashesMatch("1105", "870880", "h1", "h1b")
	if err != nil {
		t.Fatalf("CredentialHashesMatch() error = %v", err)
	}
	if !match {
		t.Error("match = false, want true")
	}
}

func TestCredentialHashesMatch_Mismatch(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<SipCredentialResponse><SipCredential>` +
			`<Id>870880</Id><Hash1>other</Hash1><Hash1b>otherb</Hash1b>` +
			`</SipCredential></SipCredentialResponse>`))
	})
	defer done()

	match, err := svc.CredentialHashesMatch("1105", "870880", "h1", "h1b")
	if err != nil {
		t.Fatalf("CredentialHashesMatch() error = %v", err)
	}
	if match {
		t.Error("match = true, want false")
	}
}

func TestCredentialHashesMatch_AbsentHashesIsError(t *testing.T) {
	// A response that decodes cleanly but carries no digest hashes at all must
	// not silently report "false" (different password) — that would tell an
	// --if-not-exists caller to rotate a credential that may be perfectly
	// correct, inverting the one contract --if-not-exists exists to provide.
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<SipCredentialResponse><SipCredential>` +
			`<Id>870880</Id><UserName>agent</UserName>` +
			`</SipCredential></SipCredentialResponse>`))
	})
	defer done()

	_, err := svc.CredentialHashesMatch("1105", "870880", "h1", "h1b")
	if err == nil {
		t.Fatal("CredentialHashesMatch() error = nil, want error for a response with no hashes")
	}
	if !strings.Contains(err.Error(), "no digest hashes") {
		t.Errorf("error = %q, want mention of missing hashes", err.Error())
	}
}

func TestCredentialHashesMatch_WrongShapedBodyIsDecodeError(t *testing.T) {
	// credentialHashWire's XMLName pins the expected root element; a
	// completely different document shape must fail to decode rather than
	// silently unmarshal into a zero-valued struct.
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<SomeOtherResponse><Foo>bar</Foo></SomeOtherResponse>`))
	})
	defer done()

	_, err := svc.CredentialHashesMatch("1105", "870880", "h1", "h1b")
	if err == nil {
		t.Fatal("CredentialHashesMatch() error = nil, want decode error for a wrong-shaped body")
	}
}

// TestParseFault_ScrubsHashEchoedInDescription covers the leak the spec records
// at line 294: the live 23026 response echoes the SUBMITTED hashes back inside
// the error text. APIFault.Error() prints Description and faultExit prints it
// again, and stderr is captured verbatim in agent transcripts and CI logs. The
// hash here is bare prose — no <Hash1> element — which is exactly what the
// element-anchored scrubber cannot see.
func TestParseFault_ScrubsHashEchoedInDescription(t *testing.T) {
	const hash = "d41d8cd98f00b204e9800998ecf8427e"
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`<SipCredentialsResponse><Errors><Error>` +
			`<ErrorCode>23026</ErrorCode>` +
			`<Description>SipCredential with Hash1 value ` + hash + ` does already exist</Description>` +
			`</Error></Errors></SipCredentialsResponse>`))
	})
	defer done()

	_, err := svc.CreateCredential("1103", "agent", hash, hash, "")
	if err == nil {
		t.Fatal("CreateCredential() error = nil, want a fault")
	}
	if strings.Contains(err.Error(), hash) {
		t.Errorf("error surfaced the digest hash: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Errorf("error missing [REDACTED] marker: %s", err.Error())
	}
	// The diagnostic content around the hash must survive.
	if !strings.Contains(err.Error(), "23026") || !strings.Contains(err.Error(), "does already exist") {
		t.Errorf("scrubbing destroyed the diagnostic content: %s", err.Error())
	}
}

// TestDo_TruncatedBodyIsDiscardedNotPartiallyScrubbed covers the fail-open path
// the spec closes at line 293: "malformed or truncated XML error bodies are
// discarded entirely rather than partially scrubbed — if it can't be parsed, it
// can't be proven hash-free."
//
// A body truncated mid-hash has no closing tag, so hashElementRe cannot match
// it; before the fix the value passed through completely unredacted.
func TestDo_TruncatedBodyIsDiscardedNotPartiallyScrubbed(t *testing.T) {
	const hash = "d41d8cd98f00b204e9800998ecf8427e"
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		// Connection died mid-element: no </Hash1>, no closing ancestors.
		w.Write([]byte(`<SipCredentialsResponse><Errors><Error><ErrorCode>23026</ErrorCode>` +
			`<SipCredential><Hash1>` + hash))
	})
	defer done()

	_, err := svc.GetRealm("1103")
	if err == nil {
		t.Fatal("GetRealm() error = nil, want an error")
	}
	if strings.Contains(err.Error(), hash) {
		t.Errorf("truncated body leaked the digest hash: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "discarded") {
		t.Errorf("error = %q, want the discarded-body placeholder", err.Error())
	}
}

// TestDo_ParsedBodyWithoutErrorCodeKeepsItsBody is the negative pairing: the
// live 404 shape is a ResponseStatus carrying a Description and NO ErrorCode.
// It parses fine, so its body is provably hash-free and useful — discarding it
// would throw away the only diagnostic the caller gets.
func TestDo_ParsedBodyWithoutErrorCodeKeepsItsBody(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`<RealmResponse><ResponseStatus>` +
			`<Description>The realm 9999 was not found</Description>` +
			`</ResponseStatus></RealmResponse>`))
	})
	defer done()

	_, err := svc.GetRealm("9999")
	if err == nil {
		t.Fatal("GetRealm() error = nil, want a 404")
	}
	if !strings.Contains(err.Error(), "was not found") {
		t.Errorf("error = %q, want the parseable 404 body preserved", err.Error())
	}
	if strings.Contains(err.Error(), "discarded") {
		t.Errorf("error = %q, want the body kept — it parsed, so it is provably hash-free", err.Error())
	}
}

// TestListCredentials_FullPageWarnsAboutTruncation covers the silent cap.
// The spec promises auto-pagination; it is not implemented. A list that returns
// exactly the API's page size therefore reads as "complete" when it may not be.
// Pagination stays deferred, but the silence does not.
func TestListCredentials_FullPageWarnsAboutTruncation(t *testing.T) {
	var b strings.Builder
	for i := 0; i < credentialPageSize; i++ {
		b.WriteString(`<SipCredential><Id>1</Id><UserName>u</UserName></SipCredential>`)
	}
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<SipCredentialsResponse><SipCredentials>` + b.String() +
			`</SipCredentials></SipCredentialsResponse>`))
	})
	defer done()

	var warnings strings.Builder
	orig := warnOut
	warnOut = &warnings
	defer func() { warnOut = orig }()

	creds, err := svc.ListCredentials("1103")
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(creds) != credentialPageSize {
		t.Fatalf("len = %d, want %d", len(creds), credentialPageSize)
	}
	got := warnings.String()
	if !strings.Contains(got, "may be truncated") {
		t.Errorf("warning = %q, want it to state the list may be truncated", got)
	}
	if !strings.Contains(got, "Pagination is not yet implemented") {
		t.Errorf("warning = %q, want it to name the missing capability", got)
	}
}

// TestListCredentials_PartialPageDoesNotWarn is the negative pairing: a list
// that is obviously complete must stay quiet, or the warning becomes noise an
// agent learns to ignore.
func TestListCredentials_PartialPageDoesNotWarn(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<SipCredentialsResponse><SipCredentials>` +
			`<SipCredential><Id>1</Id><UserName>u</UserName></SipCredential>` +
			`</SipCredentials></SipCredentialsResponse>`))
	})
	defer done()

	var warnings strings.Builder
	orig := warnOut
	warnOut = &warnings
	defer func() { warnOut = orig }()

	if _, err := svc.ListCredentials("1103"); err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if warnings.Len() != 0 {
		t.Errorf("unexpected warning for a partial page: %q", warnings.String())
	}
}

// TestUpdateRealm_ReadModifyWritePreservesUnspecifiedFields pins the service
// contract the command depends on: realm PUT is a full replace, so any field
// the caller did not name must be echoed back from current state.
func TestUpdateRealm_ReadModifyWritePreservesUnspecifiedFields(t *testing.T) {
	tests := []struct {
		name           string
		currentDesc    string
		currentDefault bool
		promote        bool
		description    *string
		wantDesc       string
		wantDefault    bool
	}{
		{"description only keeps default", "old", true, false, strPtr("new"), "new", true},
		{"promotion keeps description", "keep", false, true, nil, "keep", true},
		{"neither re-sends current state", "keep", true, false, nil, "keep", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent string
			svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					b, _ := io.ReadAll(r.Body)
					sent = string(b)
				}
				def := "false"
				if tt.currentDefault {
					def = "true"
				}
				w.Write([]byte(`<RealmResponse><Realm><Id>1103</Id>` +
					`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm>` +
					`<Description>` + tt.currentDesc + `</Description>` +
					`<Default>` + def + `</Default><Status>ACTIVE</Status>` +
					`</Realm></RealmResponse>`))
			})
			defer done()

			if _, err := svc.UpdateRealm("vapi", tt.promote, tt.description); err != nil {
				t.Fatalf("UpdateRealm() error = %v", err)
			}
			if !strings.Contains(sent, "<Description>"+tt.wantDesc+"</Description>") {
				t.Errorf("PUT body = %q, want Description %q", sent, tt.wantDesc)
			}
			wantDefault := "<Default>false</Default>"
			if tt.wantDefault {
				wantDefault = "<Default>true</Default>"
			}
			if !strings.Contains(sent, wantDefault) {
				t.Errorf("PUT body = %q, want %s", sent, wantDefault)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
