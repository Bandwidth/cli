// golden_test.go locks the --plain output contract for `band sip` commands:
// the exact JSON shape agents parse. Every other test in this package covers
// logic; these assert what a command actually PRINTS — field presence, real
// JSON types surviving the XML layer (not stringified scalars), empty lists
// rendering as [] rather than null, and digest hashes never reaching output.
//
// Coverage already exists elsewhere in this package for: all four `band sip
// status` output shapes (status_test.go), the emitCredential password
// inclusion/omission shapes (credential_create_test.go's
// TestEmitCredential_IncludesGeneratedPasswordExactlyOnce and
// TestEmitCredential_OmitsCallerSuppliedPassword), and all faultExit /
// realmReuseAllowed / realmStateMatches paths (sip_test.go). This file does
// not duplicate those.
package sip

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/testutil"
)

// runCmd drives a real cobra command through a minimal root and returns
// everything written to stdout. t.Fatal on a non-nil Execute error, since
// every golden test below expects success.
func runCmd(t *testing.T, c *cobra.Command, args ...string) string {
	t.Helper()
	root := testutil.NewTestRoot(c)
	root.SetArgs(args)
	return testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
}

const realmXML = `<?xml version='1.0'?><RealmResponse><Realm>` +
	`<Id>1103</Id><Realm>vapi-3efeaa.auth.bandwidth.com</Realm>` +
	`<Description>d</Description><Default>false</Default>` +
	`<SipCredentialCount>2</SipCredentialCount><Status>ACTIVE</Status>` +
	`</Realm></RealmResponse>`

// TestRealmGetPlainShape guards field presence AND real JSON types: the SIP
// API is XML-only, and the older generic XML helper stringified everything.
// "default": "false" (a string) instead of false (a bool) is a live,
// realistic regression this must catch.
func TestRealmGetPlainShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(realmXML))
	}))
	defer srv.Close()
	withStubService(t, srv)

	out := runCmd(t, realmGetCmd, "get", "vapi", "--plain")

	var got map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &got); err != nil {
		t.Fatalf("not JSON: %q (%v)", out, err)
	}
	// All three identifiers must be present: any one is valid input to the
	// next command (band sip realm get/update/delete, credential list, ...).
	for _, k := range []string{"id", "name", "hostname", "default", "status", "credentialCount", "description"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing field %q in %q", k, out)
		}
	}
	if got["hostname"] != "vapi-3efeaa.auth.bandwidth.com" {
		t.Errorf("hostname = %v, want the FQDN verbatim from the API", got["hostname"])
	}
	if got["name"] != "vapi" {
		t.Errorf("name = %v, want vapi (derived short label)", got["name"])
	}
	// Booleans and counts must survive the XML layer as real types, not strings.
	if _, ok := got["default"].(bool); !ok {
		t.Errorf("default = %T (%v), want bool", got["default"], got["default"])
	}
	if _, ok := got["credentialCount"].(float64); !ok {
		t.Errorf("credentialCount = %T (%v), want number", got["credentialCount"], got["credentialCount"])
	}
}

// TestRealmListEmptyIsArrayNotNull guards the classic XML-to-JSON regression:
// an account with zero realms must render as [], not null, or every script
// piping this into jq or a JSON array iterator breaks.
func TestRealmListEmptyIsArrayNotNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<?xml version='1.0'?><RealmsResponse><Realms/></RealmsResponse>`))
	}))
	defer srv.Close()
	withStubService(t, srv)

	out := strings.TrimSpace(runCmd(t, realmListCmd, "list", "--plain"))

	var got []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("empty list is not a JSON array: %q (%v)", out, err)
	}
	if got == nil {
		t.Errorf("empty list rendered as null, want []: %q", out)
	}
}

// TestCredentialListNoHashesAndFieldShape guards two things at once: that
// Hash1/Hash1b digest material never reaches --plain output (the stub
// deliberately echoes both, so a stub without them would make this
// vacuous), and that each item carries the field set the next command
// (credential get/delete/rotate) needs.
func TestCredentialListNoHashesAndFieldShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The realm lookup credential list issues first.
		if strings.HasSuffix(r.URL.Path, "/realms/vapi") {
			w.Write([]byte(realmXML))
			return
		}
		// The live API answers an unpaginated credential list with a 303.
		if r.URL.Query().Get("page") == "" {
			http.Redirect(w, r, r.URL.Path+"?page=1&size=500", http.StatusSeeOther)
			return
		}
		w.Write([]byte(`<SipCredentialsResponse><SipCredentials><SipCredential>` +
			`<Id>870874</Id><RealmId>1103</RealmId><UserName>agent</UserName>` +
			`<Hash1>1be6abcaa8e9956021d30f33a3925b99</Hash1>` +
			`<Hash1b>e028e6577a0bb1b90a33d30a110dbdfe</Hash1b>` +
			`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm>` +
			`</SipCredential></SipCredentials></SipCredentialsResponse>`))
	}))
	defer srv.Close()
	withStubService(t, srv)

	out := runCmd(t, credentialListCmd, "list", "--realm", "vapi", "--plain")

	if strings.Contains(out, "1be6abcaa8e9956021d30f33a3925b99") ||
		strings.Contains(strings.ToLower(out), "hash1") {
		t.Fatalf("digest hash reached --plain output: %q", out)
	}
	var got []map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &got); err != nil {
		t.Fatalf("not a JSON array: %q (%v)", out, err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	for _, k := range []string{"id", "realmId", "username", "hostname", "appId"} {
		if _, ok := got[0][k]; !ok {
			t.Errorf("missing field %q in %q", k, out)
		}
	}
}

// TestRealmDeleteAcceptedShape guards the accepted-vs-completed shape without
// --wait, and that "id" is the realm's canonical ID rather than whatever
// identifier the caller happened to type. The command is invoked by NAME here
// (the form its own --help documents), so before the resolve-first fix this
// returned {"id":"vapi"} — a name in the field an agent feeds to the next
// command as an ID.
//
// The live API returns 202 with an empty body for delete; the stub matches that
// exactly and serves the realm lookup that now precedes it.
func TestRealmDeleteAcceptedShape(t *testing.T) {
	var deletePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletePath = r.URL.Path
			w.WriteHeader(202)
			return
		}
		w.Write([]byte(realmXML))
	}))
	defer srv.Close()
	withStubService(t, srv)

	out := runCmd(t, realmDeleteCmd, "delete", "vapi", "--plain")

	var got map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &got); err != nil {
		t.Fatalf("not JSON: %q (%v)", out, err)
	}
	if got["accepted"] != true {
		t.Errorf("accepted = %v, want true", got["accepted"])
	}
	if got["deleted"] != false {
		t.Errorf("deleted = %v, want false without --wait", got["deleted"])
	}
	// realmXML's realm is ID 1103, name "vapi". Invoked by name, the output must
	// still report the ID.
	if got["id"] != "1103" {
		t.Errorf("id = %v, want the resolved realm ID \"1103\" — not the %q ref the caller passed", got["id"], "vapi")
	}
	// The DELETE itself must go to the resolved ID too, so the polling loop and
	// the reported ID describe the same resource.
	if !strings.HasSuffix(deletePath, "/realms/1103") {
		t.Errorf("DELETE path = %q, want it to end in /realms/1103", deletePath)
	}
}
