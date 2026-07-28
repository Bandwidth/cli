package sip

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	sipsvc "github.com/Bandwidth/cli/internal/sip"
	"github.com/Bandwidth/cli/internal/testutil"
)

// credentialCreateStubServer returns a fault-injectable stub for the requests
// runCredentialCreate's --if-not-exists path issues: GetRealm, the duplicate
// CreateCredential, ListCredentials (FindCredentialByUsername), and the
// re-read GetCredential (the app-binding guard added after code review).
func credentialCreateStubServer(t *testing.T, appID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/realms/vapi"):
			w.Write([]byte(`<RealmResponse><Realm><Id>1103</Id>` +
				`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm><Status>ACTIVE</Status>` +
				`</Realm></RealmResponse>`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sipcredentials"):
			w.WriteHeader(201)
			w.Write([]byte(`<SipCredentialsResponse><Errors><Error>` +
				`<ErrorCode>23026</ErrorCode><Description>does already exist</Description>` +
				`</Error></Errors></SipCredentialsResponse>`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sipcredentials"):
			w.Write([]byte(`<SipCredentialsResponse><SipCredentials><SipCredential>` +
				`<Id>55</Id><UserName>agent</UserName></SipCredential>` +
				`</SipCredentials></SipCredentialsResponse>`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/sipcredentials/"):
			w.Write([]byte(`<SipCredentialResponse><SipCredential>` +
				`<Id>55</Id><UserName>agent</UserName><HttpVoiceV2AppId>` + appID + `</HttpVoiceV2AppId>` +
				`</SipCredential></SipCredentialResponse>`))
		default:
			w.WriteHeader(404)
		}
	}))
}

func withStubService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := service
	t.Cleanup(func() { service = orig })
	service = func(cmd *cobra.Command) (*sipsvc.Service, error) {
		return sipsvc.NewService(api.NewXMLClient(srv.URL, nil), "9901361"), nil
	}
}

// TestCredentialCreate_IfNotExistsGeneratedPasswordOnExisting_ExitsSecretUnavailable
// guards the security invariant reviewed as most load-bearing in this
// command: an agent must never be told "success" for a generated password it
// cannot recover. --if-not-exists against an existing credential, combined
// with --generate-password, must exit 8 (ExitSecretUnavailable), not 0.
func TestCredentialCreate_IfNotExistsGeneratedPasswordOnExisting_ExitsSecretUnavailable(t *testing.T) {
	srv := credentialCreateStubServer(t, "")
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(credentialCreateCmd)
	root.SetArgs([]string{"create", "--realm", "vapi", "--username", "agent", "--generate-password", "--if-not-exists"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want SecretUnavailableError")
	}
	if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitSecretUnavailable {
		t.Errorf("ExitCodeForError() = %d, want %d (ExitSecretUnavailable); err = %v", got, cmdutil.ExitSecretUnavailable, err)
	}
}

// TestCredentialCreate_IfNotExistsAppMismatch_ReportsNotBound covers the
// re-read-before-compare fix: the app-binding gate must not trust
// ListCredentials' shape for HttpVoiceV2AppId, and an empty existing binding
// must be reported distinctly ("not bound") rather than as a generic
// different-application mismatch.
func TestCredentialCreate_IfNotExistsAppMismatch_ReportsNotBound(t *testing.T) {
	srv := credentialCreateStubServer(t, "") // existing credential is unbound
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(credentialCreateCmd)
	root.SetArgs([]string{"create", "--realm", "vapi", "--username", "agent", "--app-id", "app-123", "--generate-password", "--if-not-exists"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want an app-binding mismatch error")
	}
	if !strings.Contains(err.Error(), "not bound to an application") {
		t.Errorf("error = %q, want mention of the credential not being bound", err.Error())
	}
}

// TestEmitCredential_OmitsCallerSuppliedPassword is the most security-load-bearing
// assertion in this package: a caller-supplied password must never be echoed
// back, and passwordShownOnce must be false whenever password is absent.
func TestEmitCredential_OmitsCallerSuppliedPassword(t *testing.T) {
	cred := &sipsvc.Credential{ID: "1", RealmID: "10", Username: "agent", Hostname: "vapi.example.com"}
	wrap := &cobra.Command{
		Use: "wrap",
		RunE: func(cmd *cobra.Command, args []string) error {
			return emitCredential(cmd, cred, "callerSuppliedSecret", false)
		},
	}
	root := testutil.NewTestRoot(wrap)
	root.SetArgs([]string{"wrap", "--plain"})

	out := testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %q (%v)", out, err)
	}
	if _, present := got["password"]; present {
		t.Errorf("output contains a \"password\" key for a caller-supplied secret: %q", out)
	}
	if v, ok := got["passwordShownOnce"].(bool); !ok || v {
		t.Errorf("passwordShownOnce = %v, want false", got["passwordShownOnce"])
	}
	if strings.Contains(out, "callerSuppliedSecret") {
		t.Errorf("output leaked the caller-supplied password: %q", out)
	}
}

// TestEmitCredential_IncludesGeneratedPasswordExactlyOnce is the counterpart
// to the omission test: a generated password must be present exactly when
// generated=true, alongside passwordShownOnce=true.
func TestEmitCredential_IncludesGeneratedPasswordExactlyOnce(t *testing.T) {
	cred := &sipsvc.Credential{ID: "1", RealmID: "10", Username: "agent", Hostname: "vapi.example.com"}
	wrap := &cobra.Command{
		Use: "wrap",
		RunE: func(cmd *cobra.Command, args []string) error {
			return emitCredential(cmd, cred, "generatedSecret123", true)
		},
	}
	root := testutil.NewTestRoot(wrap)
	root.SetArgs([]string{"wrap", "--plain"})

	out := testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %q (%v)", out, err)
	}
	if got["password"] != "generatedSecret123" {
		t.Errorf("password = %v, want generatedSecret123", got["password"])
	}
	if v, ok := got["passwordShownOnce"].(bool); !ok || !v {
		t.Errorf("passwordShownOnce = %v, want true", got["passwordShownOnce"])
	}
}
