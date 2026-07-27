package sip

import (
	"errors"
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
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
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
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
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
	// A 201 can still carry an Errors array; the requested credential must be
	// present in ValidSipCredentials or the command fails.
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
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

// errorsAs keeps the assertions above readable.
func errorsAs(err error, target interface{}) bool {
	return errors.As(err, target)
}
