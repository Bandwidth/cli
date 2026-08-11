package sip

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/testutil"
)

// resetRealmUpdateFlags restores realmUpdateCmd's flags to their unset state.
// Flag values and their Changed bits are package/command state that cobra only
// assigns on parse, so without this a value from an earlier test in the same
// process leaks into the next one — and `Changed` is exactly what this command
// branches on.
func resetRealmUpdateFlags(t *testing.T) {
	t.Helper()
	for _, name := range []string{"default", "description"} {
		fl := realmUpdateCmd.Flags().Lookup(name)
		if err := fl.Value.Set(fl.DefValue); err != nil {
			t.Fatalf("resetting --%s: %v", name, err)
		}
		fl.Changed = false
	}
}

// capturedRealmPUT records the body of the PUT `sip realm update` issues, so a
// test can assert what was actually SENT — the whole point of read-modify-write
// is that an omitted field is re-transmitted, which output alone cannot prove.
type capturedRealmPUT struct {
	Realm       string `xml:"Realm"`
	Description string `xml:"Description"`
	Default     bool   `xml:"Default"`
}

// realmUpdateStub serves a realm with the given current state and captures the
// PUT body. The PUT response echoes the request, mimicking the live API.
func realmUpdateStub(t *testing.T, currentDesc string, currentDefault bool, got *capturedRealmPUT) *httptest.Server {
	t.Helper()
	def := "false"
	if currentDefault {
		def = "true"
	}
	current := `<?xml version='1.0'?><RealmResponse><Realm><Id>1103</Id>` +
		`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm><Description>` + currentDesc + `</Description>` +
		`<Default>` + def + `</Default><SipCredentialCount>0</SipCredentialCount>` +
		`<Status>ACTIVE</Status></Realm></RealmResponse>`

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(current))
		case http.MethodPut:
			var body struct {
				XMLName     xml.Name `xml:"Realm"`
				Realm       string   `xml:"Realm"`
				Description string   `xml:"Description"`
				Default     bool     `xml:"Default"`
			}
			buf := make([]byte, 4096)
			n, _ := r.Body.Read(buf)
			if err := xml.Unmarshal(buf[:n], &body); err != nil {
				t.Errorf("PUT body is not the expected Realm shape: %v (%q)", err, buf[:n])
			}
			got.Realm, got.Description, got.Default = body.Realm, body.Description, body.Default

			respDefault := "false"
			if body.Default {
				respDefault = "true"
			}
			w.Write([]byte(`<?xml version='1.0'?><RealmResponse><Realm><Id>1103</Id>` +
				`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm><Description>` + body.Description + `</Description>` +
				`<Default>` + respDefault + `</Default><SipCredentialCount>0</SipCredentialCount>` +
				`<Status>ACTIVE</Status></Realm></RealmResponse>`))
		default:
			w.WriteHeader(404)
		}
	}))
}

// TestRealmUpdate_DescriptionOnlyPreservesDefault covers the half of the
// command that did not exist. `sip realm create --if-not-exists` tells a caller
// on a description mismatch to "update it explicitly", and AGENTS.md repeats it
// — but --description was never implemented, so no command could carry out the
// CLI's own remediation. Realm PUT is a full replace, so this also asserts the
// unspecified `default` is echoed back rather than cleared.
func TestRealmUpdate_DescriptionOnlyPreservesDefault(t *testing.T) {
	var got capturedRealmPUT
	srv := realmUpdateStub(t, "old", true, &got) // currently the DEFAULT realm
	defer srv.Close()
	withStubService(t, srv)

	resetRealmUpdateFlags(t)
	root := testutil.NewTestRoot(realmUpdateCmd)
	root.SetArgs([]string{"update", "vapi", "--description", "new text", "--plain"})

	testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if got.Description != "new text" {
		t.Errorf("PUT Description = %q, want %q", got.Description, "new text")
	}
	if !got.Default {
		t.Error("PUT Default = false, want true — a description-only update must not demote the default realm")
	}
}

// TestRealmUpdate_DefaultOnlyPreservesDescription is the mirror: promotion must
// not wipe the description out from under the caller.
func TestRealmUpdate_DefaultOnlyPreservesDescription(t *testing.T) {
	var got capturedRealmPUT
	srv := realmUpdateStub(t, "keep me", false, &got)
	defer srv.Close()
	withStubService(t, srv)

	resetRealmUpdateFlags(t)
	root := testutil.NewTestRoot(realmUpdateCmd)
	// --description is deliberately absent: that is the case under test.
	root.SetArgs([]string{"update", "vapi", "--default=true", "--plain"})

	testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if !got.Default {
		t.Error("PUT Default = false, want true")
	}
	if got.Description != "keep me" {
		t.Errorf("PUT Description = %q, want the existing %q re-sent — realm PUT is a full replace", got.Description, "keep me")
	}
}

// TestRealmUpdate_RejectsDefaultFalse keeps the pre-existing guard: the API
// cannot demote a realm, so --default=false must fail loudly rather than
// silently no-op. Making --default optional must not weaken this.
func TestRealmUpdate_RejectsDefaultFalse(t *testing.T) {
	resetRealmUpdateFlags(t)
	root := testutil.NewTestRoot(realmUpdateCmd)
	root.SetArgs([]string{"update", "vapi", "--default=false"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want --default=false to be rejected")
	}
	if !strings.Contains(err.Error(), "--default=false is not supported by the API") {
		t.Errorf("error = %q, want the existing rejection message", err.Error())
	}
}

// TestRealmUpdate_RequiresAtLeastOneField guards the flag contract now that
// --default is no longer strictly required: an argument-only invocation must
// not silently issue a PUT that changes nothing.
func TestRealmUpdate_RequiresAtLeastOneField(t *testing.T) {
	resetRealmUpdateFlags(t)
	root := testutil.NewTestRoot(realmUpdateCmd)
	root.SetArgs([]string{"update", "vapi"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want a flag error")
	}
	if !strings.Contains(err.Error(), "at least one of --default=true or --description") {
		t.Errorf("error = %q, want guidance naming both flags", err.Error())
	}
}
