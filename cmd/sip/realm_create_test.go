package sip

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/testutil"
)

// realmListXML renders a one-realm list response.
func realmListXML(name, status string, isDefault bool) string {
	def := "false"
	if isDefault {
		def = "true"
	}
	return `<?xml version='1.0'?><RealmsResponse><Realms><Realm>` +
		`<Id>1103</Id><Realm>` + name + `-3efeaa.auth.bandwidth.com</Realm>` +
		`<Description></Description><Default>` + def + `</Default>` +
		`<SipCredentialCount>0</SipCredentialCount><Status>` + status + `</Status>` +
		`</Realm></Realms></RealmsResponse>`
}

// realmGetXML renders a single-realm response.
func realmGetXML(name, status string) string {
	return `<?xml version='1.0'?><RealmResponse><Realm>` +
		`<Id>1103</Id><Realm>` + name + `-3efeaa.auth.bandwidth.com</Realm>` +
		`<Description></Description><Default>false</Default>` +
		`<SipCredentialCount>0</SipCredentialCount><Status>` + status + `</Status>` +
		`</Realm></RealmResponse>`
}

// TestRealmCreate_IfNotExistsWithWaitPollsPendingRealmToActive covers the gap
// between AGENTS.md's Timeout Recovery advice and the code. That table tells an
// agent that after a `--wait` timeout, "re-running create with --if-not-exists
// is safe" — so the agent re-runs with BOTH flags. realmReuseAllowed admits
// CREATE_PENDING, so the reuse path used to return immediately, handing back
// exit 0 with status CREATE_PENDING. The agent reads success and the next step
// (credential create, which requires ACTIVE) fails.
func TestRealmCreate_IfNotExistsWithWaitPollsPendingRealmToActive(t *testing.T) {
	var gets int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		// The --if-not-exists lookup sees a realm still provisioning.
		case strings.HasSuffix(r.URL.Path, "/realms"):
			w.Write([]byte(realmListXML("vapi", "CREATE_PENDING", false)))
		// The poll then observes it reach ACTIVE.
		case strings.Contains(r.URL.Path, "/realms/"):
			atomic.AddInt32(&gets, 1)
			w.Write([]byte(realmGetXML("vapi", "ACTIVE")))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(realmCreateCmd)
	root.SetArgs([]string{"create", "--name", "vapi", "--default=false", "--if-not-exists", "--wait", "--timeout", "10", "--plain"})

	out := testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if atomic.LoadInt32(&gets) == 0 {
		t.Error("--wait was ignored on the --if-not-exists reuse path: no realm GET was issued")
	}
	var got map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &got); err != nil {
		t.Fatalf("not JSON: %q (%v)", out, err)
	}
	if got["status"] != "ACTIVE" {
		t.Errorf("status = %v, want ACTIVE — a reused CREATE_PENDING realm with --wait must be polled to ACTIVE before returning", got["status"])
	}
}

// TestRealmCreate_IfNotExistsRejectsUppercaseName verifies that uppercase realm
// names are rejected before any API call. The API only accepts [a-z0-9] (error
// 33013), so validation must catch this early — an uppercase name that reaches
// the server would fail unpredictably and potentially burn a generated password.
func TestRealmCreate_IfNotExistsRejectsUppercaseName(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(500)
	}))
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(realmCreateCmd)
	root.SetArgs([]string{"create", "--name", "VAPI", "--default=false", "--if-not-exists", "--plain"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil, want error for uppercase realm name")
	}
	if n := atomic.LoadInt32(&requests); n != 0 {
		t.Errorf("issued %d HTTP requests — validation must reject uppercase names before any API call", n)
	}
}

// TestRealmCreate_IfNotExistsStateMismatchExitsConflict is the command-level
// half of the exit-code fix on the `sip` side: a client-side state conflict is
// a conflict (4), not a generic failure (1). An agent branching on 1 vs 4
// cannot otherwise tell "the realm exists but differs, go update it" from "the
// CLI broke."
func TestRealmCreate_IfNotExistsStateMismatchExitsConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/realms") && r.Method == http.MethodGet {
			w.Write([]byte(realmListXML("vapi", "ACTIVE", false)))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(realmCreateCmd)
	// The existing realm is default=false; ask for default=true.
	root.SetArgs([]string{"create", "--name", "vapi", "--default=true", "--if-not-exists", "--wait=false"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want a state-mismatch conflict")
	}
	if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitConflict {
		t.Errorf("ExitCodeForError() = %d, want ExitConflict (%d); err = %v", got, cmdutil.ExitConflict, err)
	}
	if !strings.Contains(err.Error(), "update it explicitly") {
		t.Errorf("error = %q, want the existing remediation text preserved verbatim", err.Error())
	}
}
