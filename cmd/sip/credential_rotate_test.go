package sip

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/testutil"
)

// credentialRotatePostWriteFailureStubServer simulates a failure on the
// RotateCredential (PUT) call itself — e.g. a 500 on the response leg of a
// PUT that may already have replaced the hashes server-side. This is the
// scenario code review flagged: the write may have landed, the generated
// password was never printed, and a working SIP peer's credential may now be
// silently dead with an unrecoverable password.
func credentialRotatePostWriteFailureStubServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/realms/vapi"):
			w.Write([]byte(`<RealmResponse><Realm><Id>1105</Id>` +
				`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm><Status>ACTIVE</Status>` +
				`</Realm></RealmResponse>`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/sipcredentials/"):
			w.Write([]byte(`<SipCredentialResponse><SipCredential>` +
				`<Id>870880</Id><UserName>rotuser</UserName>` +
				`</SipCredential></SipCredentialResponse>`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/sipcredentials/"):
			w.WriteHeader(500)
			w.Write([]byte("boom"))
		default:
			w.WriteHeader(404)
		}
	}))
}

// TestCredentialRotate_GeneratedPasswordLostToPostWriteFailure_ExitsSecretUnavailable
// exercises the branch code review found untested: RotateCredential returning
// an error while generated == true (credential_rotate.go's RunE, in the
// post-RotateCredential error handling). The generated password was never
// printed, so this must surface as *cmdutil.SecretUnavailableError (exit 8),
// not a generic, retryable-looking failure.
func TestCredentialRotate_GeneratedPasswordLostToPostWriteFailure_ExitsSecretUnavailable(t *testing.T) {
	srv := credentialRotatePostWriteFailureStubServer(t)
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(credentialRotateCmd)
	root.SetArgs([]string{"rotate", "870880", "--realm", "vapi", "--generate-password"})

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

// TestCredentialRotate_CallerSuppliedPasswordLostToPostWriteFailure_FallsThroughToFaultExit
// is the negative pairing: the identical failure with a caller-supplied
// password (--password-stdin) must NOT produce SecretUnavailableError — the
// caller already knows the password, so there is nothing unrecoverable. This
// proves the branch keys on `generated`, not on the failure alone.
func TestCredentialRotate_CallerSuppliedPasswordLostToPostWriteFailure_FallsThroughToFaultExit(t *testing.T) {
	srv := credentialRotatePostWriteFailureStubServer(t)
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(credentialRotateCmd)
	root.SetIn(strings.NewReader("hunter2\n"))
	root.SetArgs([]string{"rotate", "870880", "--realm", "vapi", "--password-stdin"})

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
