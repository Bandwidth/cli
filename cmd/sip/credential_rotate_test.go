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

// credentialRotateFaultStubServer is like the above but the PUT is rejected
// with a structured Bandwidth fault at the given status — the server parsed the
// request and refused it, so the hashes were definitively NOT replaced.
func credentialRotateFaultStubServer(t *testing.T, status int, code string) *httptest.Server {
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
			w.WriteHeader(status)
			w.Write([]byte(`<SipCredentialResponse><ResponseStatus>` +
				`<ErrorCode>` + code + `</ErrorCode><Description>rejected</Description>` +
				`</ResponseStatus></SipCredentialResponse>`))
		default:
			w.WriteHeader(404)
		}
	}))
}

// TestCredentialRotate_DefinitiveFaultDoesNotExitSecretUnavailable is the
// discrimination this branch was missing. Exit 8 means "a credential you cannot
// use may now exist — rotate it." That is false for any *APIFault: the server
// parsed and rejected the request, so no write happened. A 429 must report exit
// 7 (back off and retry) exactly as --password-stdin would from the same
// response; blanket exit 8 sent the agent down an unrecoverable-secret path for
// a plainly retryable failure.
func TestCredentialRotate_DefinitiveFaultDoesNotExitSecretUnavailable(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   string
		want   int
	}{
		{"rate limited", 429, "1001", cmdutil.ExitRateLimit},
		{"documented 400 conflict", 400, "23022", cmdutil.ExitConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := credentialRotateFaultStubServer(t, c.status, c.code)
			defer srv.Close()
			withStubService(t, srv)

			root := testutil.NewTestRoot(credentialRotateCmd)
			// Every password flag is set explicitly: they are package vars on a
			// shared command and cobra only assigns on parse, so a value left
			// behind by an earlier test in this process would trip the
			// "exactly one password source" guard first.
			root.SetArgs([]string{"rotate", "870880", "--realm", "vapi",
				"--generate-password", "--password-stdin=false", "--password-file="})

			err := root.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want the fault to surface")
			}
			var sue *cmdutil.SecretUnavailableError
			if errors.As(err, &sue) {
				t.Fatalf("error = %v, want NOT *cmdutil.SecretUnavailableError: a parsed rejection means nothing was written", err)
			}
			if got := cmdutil.ExitCodeForError(err); got != c.want {
				t.Errorf("ExitCodeForError() = %d, want %d; err = %v", got, c.want, err)
			}
		})
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
