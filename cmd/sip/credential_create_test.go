package sip

import (
	"encoding/json"
	"errors"
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

// credentialCreatePostWriteFailureStubServer simulates a failure on the
// CreateCredential call itself that is NOT the documented 23026 duplicate —
// e.g. a 500 on the response leg of a POST that may already have committed
// server-side. This is the scenario code review flagged: the write may have
// landed, the generated password was never printed, and the CLI must not
// report a generic, retryable-looking failure.
func credentialCreatePostWriteFailureStubServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/realms/vapi"):
			w.Write([]byte(`<RealmResponse><Realm><Id>1103</Id>` +
				`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm><Status>ACTIVE</Status>` +
				`</Realm></RealmResponse>`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sipcredentials"):
			w.WriteHeader(500)
			w.Write([]byte("boom"))
		default:
			w.WriteHeader(404)
		}
	}))
}

// TestCredentialCreate_GeneratedPasswordLostToPostWriteFailure_ExitsSecretUnavailable
// exercises the branch code review found untested: CreateCredential returning
// an error while generated == true (credential_create.go, inside
// runCredentialCreate's post-CreateCredential error handling, after the
// --if-not-exists/23026 branch is ruled out). The generated password was
// never printed, so this must surface as *cmdutil.SecretUnavailableError
// (exit 8), not a generic failure an agent might blindly retry.
func TestCredentialCreate_GeneratedPasswordLostToPostWriteFailure_ExitsSecretUnavailable(t *testing.T) {
	srv := credentialCreatePostWriteFailureStubServer(t)
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(credentialCreateCmd)
	root.SetArgs([]string{"create", "--realm", "vapi", "--username", "agent", "--generate-password"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want *cmdutil.SecretUnavailableError")
	}
	var sue *cmdutil.SecretUnavailableError
	if !errors.As(err, &sue) {
		t.Fatalf("error = %v (%T), want *cmdutil.SecretUnavailableError", err, err)
	}
	if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitSecretUnavailable {
		t.Errorf("ExitCodeForError() = %d, want %d (ExitSecretUnavailable)", got, cmdutil.ExitSecretUnavailable)
	}
}

// TestCredentialCreate_CallerSuppliedPasswordLostToPostWriteFailure_FallsThroughToFaultExit
// is the negative pairing: the identical failure with a caller-supplied
// password (--password-stdin) must NOT produce SecretUnavailableError — the
// caller already knows the password, so there is nothing unrecoverable. This
// proves the branch keys on `generated`, not on the failure alone.
func TestCredentialCreate_CallerSuppliedPasswordLostToPostWriteFailure_FallsThroughToFaultExit(t *testing.T) {
	srv := credentialCreatePostWriteFailureStubServer(t)
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(credentialCreateCmd)
	root.SetIn(strings.NewReader("hunter2\n"))
	root.SetArgs([]string{"create", "--realm", "vapi", "--username", "agent", "--password-stdin"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want an error for the failed write")
	}
	var sue *cmdutil.SecretUnavailableError
	if errors.As(err, &sue) {
		t.Fatalf("error = %v, want a plain faultExit error, NOT *cmdutil.SecretUnavailableError, for a caller-supplied password", err)
	}
	if got := cmdutil.ExitCodeForError(err); got == cmdutil.ExitSecretUnavailable {
		t.Errorf("ExitCodeForError() = %d, want anything but ExitSecretUnavailable for a caller-supplied password", got)
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
