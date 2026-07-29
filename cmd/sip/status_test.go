package sip

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/testutil"
)

// statusStubServer returns a stub that answers GET /realms (the cheap probe
// `band sip status` issues) either with a successful realms list or with the
// documented 33004 "account not enabled for SIP" fault, depending on
// wantAccountNotEnabled.
func statusStubServer(t *testing.T, wantAccountNotEnabled bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/realms") {
			w.WriteHeader(404)
			return
		}
		if wantAccountNotEnabled {
			w.WriteHeader(400)
			w.Write([]byte(`<RealmsResponse><ResponseStatus><ErrorCode>33004</ErrorCode>` +
				`<Description>Your account isn't setup for Sip Credentials</Description>` +
				`</ResponseStatus></RealmsResponse>`))
			return
		}
		w.Write([]byte(`<RealmsResponse><Realms><Realm><Id>1103</Id>` +
			`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm><Status>ACTIVE</Status>` +
			`</Realm></Realms></RealmsResponse>`))
	}))
}

// TestSIPStatus_ProbeSucceeded covers the 200 path: a successful probe reports
// status=available with the stable reason "probe_succeeded", and — critically
// for scripts/agents — exits 0 (no error returned).
func TestSIPStatus_ProbeSucceeded(t *testing.T) {
	srv := statusStubServer(t, false)
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(statusCmd)
	root.SetArgs([]string{"status", "--plain"})

	var out string
	var err error
	out = testutil.CaptureStdout(t, func() {
		err = root.Execute()
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	var got map[string]string
	if unmarshalErr := json.Unmarshal([]byte(out), &got); unmarshalErr != nil {
		t.Fatalf("output is not JSON: %q (%v)", out, unmarshalErr)
	}
	if got["status"] != "available" {
		t.Errorf("status = %q, want %q", got["status"], "available")
	}
	if got["reason"] != "probe_succeeded" {
		t.Errorf("reason = %q, want the stable identifier %q", got["reason"], "probe_succeeded")
	}
}

// TestSIPStatus_AccountNotEnabled covers the 33004 path: this is a successful
// probe reporting a negative fact about account configuration, not a command
// failure, so it must exit 0 (err == nil) even though status is "unavailable".
func TestSIPStatus_AccountNotEnabled(t *testing.T) {
	srv := statusStubServer(t, true)
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(statusCmd)
	root.SetArgs([]string{"status", "--plain"})

	var out string
	var err error
	out = testutil.CaptureStdout(t, func() {
		err = root.Execute()
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (a 33004 probe result is not a command failure)", err)
	}

	var got map[string]string
	if unmarshalErr := json.Unmarshal([]byte(out), &got); unmarshalErr != nil {
		t.Fatalf("output is not JSON: %q (%v)", out, unmarshalErr)
	}
	if got["status"] != "unavailable" {
		t.Errorf("status = %q, want %q", got["status"], "unavailable")
	}
	if got["reason"] != "account_not_enabled" {
		t.Errorf("reason = %q, want the stable identifier %q", got["reason"], "account_not_enabled")
	}
}
