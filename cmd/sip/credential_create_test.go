package sip

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
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

// testAppID is a syntactically valid voice application UUID. The fixture used
// to be "app-123", which the API would have rejected: --app-id is pinned to a
// UUID and is now validated client-side, so the old fixture could never have
// exercised the app-binding path against the live service.
const testAppID = "04e88489-df02-4e34-a0e2-4d0e0d3f7a1c"

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
	root.SetArgs([]string{"create", "--realm", "vapi", "--username", "agent", "--app-id", testAppID, "--generate-password", "--if-not-exists"})

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

// credentialCreateSuccessStubServer answers the realm lookup and returns a
// successful bulk-create response, so the command reaches its output write.
func credentialCreateSuccessStubServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/realms/vapi"):
			w.Write([]byte(`<RealmResponse><Realm><Id>1103</Id>` +
				`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm><Status>ACTIVE</Status>` +
				`</Realm></RealmResponse>`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sipcredentials"):
			w.WriteHeader(201)
			w.Write([]byte(`<SipCredentialsResponse><ValidSipCredentials><SipCredential>` +
				`<Id>870874</Id><RealmId>1103</RealmId><UserName>agent</UserName>` +
				`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm>` +
				`</SipCredential></ValidSipCredentials></SipCredentialsResponse>`))
		default:
			w.WriteHeader(404)
		}
	}))
}

// withFailingStdout points os.Stdout at an already-closed descriptor for the
// duration of a test, so every write to it fails before a single byte — the
// password included — reaches the caller. That is the spec's write-once hazard
// in its sharpest form: the POST/PUT has already committed and stdout is gone.
//
// The file is closed rather than opened O_RDONLY because Go's *os.File tracks
// closure itself and returns ErrClosed without consulting the OS, which makes
// the failure identical on every platform. An O_RDONLY descriptor only fails on
// Unix; on Windows the write succeeds and the test proves nothing.
func withFailingStdout(t *testing.T) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	orig := os.Stdout
	os.Stdout = f
	t.Cleanup(func() { os.Stdout = orig })
	// Guard against a platform where writing to a closed file succeeds: the whole
	// test would silently prove nothing.
	if _, err := f.Write([]byte("x")); err == nil {
		t.Fatal("writes to the injected stdout succeeded; this test cannot detect an output-write failure")
	}
}

// TestCredentialCreate_GeneratedPasswordLostToStdoutFailure_ExitsSecretUnavailable
// covers the gap between "the API call succeeded" and "the caller has the
// secret". The POST committed, so the credential exists; the write of the only
// copy of its password failed. Before this fix emitCredential's error travelled
// out as a generic error and mapped to exit 1, which an agent reads as "nothing
// happened, retry" — while a credential it can never authenticate with sits on
// the realm.
func TestCredentialCreate_GeneratedPasswordLostToStdoutFailure_ExitsSecretUnavailable(t *testing.T) {
	srv := credentialCreateSuccessStubServer(t)
	defer srv.Close()
	withStubService(t, srv)
	withFailingStdout(t)

	root := testutil.NewTestRoot(credentialCreateCmd)
	root.SetArgs([]string{"create", "--realm", "vapi", "--username", "agent", "--plain",
		"--generate-password", "--password-stdin=false", "--password-file=", "--if-not-exists=false", "--app-id="})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want *cmdutil.SecretUnavailableError for a failed password write")
	}
	var sue *cmdutil.SecretUnavailableError
	if !errors.As(err, &sue) {
		t.Fatalf("error = %v (%T), want *cmdutil.SecretUnavailableError", err, err)
	}
	if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitSecretUnavailable {
		t.Errorf("ExitCodeForError() = %d, want %d (ExitSecretUnavailable)", got, cmdutil.ExitSecretUnavailable)
	}
	// The recovery path must be actionable: the ID was never printed, so the
	// message has to point at list-then-rotate.
	for _, want := range []string{"band sip credential list --realm vapi --plain", "band sip credential rotate"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to name %q", err.Error(), want)
		}
	}
	// The error goes to stderr and into agent transcripts; it must not carry the
	// generated password itself.
	if strings.Contains(err.Error(), "passwordShownOnce") {
		t.Errorf("error = %q, unexpectedly echoes the payload", err.Error())
	}
}

// TestCredentialCreate_CallerSuppliedPasswordLostToStdoutFailure_ReturnsWriteError
// is the negative pairing: with --password-stdin the caller already holds the
// secret, so a stdout failure is an ordinary I/O error. Reporting exit 8 there
// would send an agent to rotate a credential that is perfectly usable.
func TestCredentialCreate_CallerSuppliedPasswordLostToStdoutFailure_ReturnsWriteError(t *testing.T) {
	srv := credentialCreateSuccessStubServer(t)
	defer srv.Close()
	withStubService(t, srv)
	withFailingStdout(t)

	root := testutil.NewTestRoot(credentialCreateCmd)
	root.SetIn(strings.NewReader("hunter2\n"))
	root.SetArgs([]string{"create", "--realm", "vapi", "--username", "agent", "--plain",
		"--password-stdin", "--generate-password=false", "--password-file=", "--if-not-exists=false", "--app-id="})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want the stdout write error to surface")
	}
	var sue *cmdutil.SecretUnavailableError
	if errors.As(err, &sue) {
		t.Fatalf("error = %v, want the plain write error, NOT *cmdutil.SecretUnavailableError: the caller supplied the password", err)
	}
	if got := cmdutil.ExitCodeForError(err); got == cmdutil.ExitSecretUnavailable {
		t.Errorf("ExitCodeForError() = %d, want anything but ExitSecretUnavailable", got)
	}
}

// TestCredentialCreate_InvalidAppIDMakesNoRequests pins the cost of a bad
// --app-id at zero: no realm lookup, and — the part that matters — no password
// generation, so a typo or a documentation placeholder can never put the caller
// on the write-once path.
func TestCredentialCreate_InvalidAppIDMakesNoRequests(t *testing.T) {
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(500)
	}))
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(credentialCreateCmd)
	root.SetArgs([]string{"create", "--realm", "vapi", "--username", "agent", "--app-id", "app-123",
		"--generate-password", "--password-stdin=false", "--password-file=", "--if-not-exists=false"})
	// --app-id is a package var on a shared cobra command, so the invalid value
	// this test sets must not leak into whatever runs next.
	t.Cleanup(func() { credCreateAppID = "" })

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want a validation error for a non-UUID --app-id")
	}
	if !strings.Contains(err.Error(), "must be a UUID") {
		t.Errorf("error = %q, want it to say --app-id must be a UUID", err.Error())
	}
	if got := atomic.LoadInt32(&requests); got != 0 {
		t.Errorf("HTTP requests = %d, want 0 — an invalid --app-id must be rejected before any API call", got)
	}
}

// TestEmitCredential_WritesGeneratedPasswordFirst pins the spec's write-once
// mitigation: the generated password is the FIRST thing written on success, so a
// stdout write truncated part-way still delivers the only copy of the secret.
// The general emit path normalizes payloads to a map and Go's encoder writes map
// keys alphabetically, which silently put appId, hostname, and id ahead of
// password.
func TestEmitCredential_WritesGeneratedPasswordFirst(t *testing.T) {
	cred := &sipsvc.Credential{ID: "870874", RealmID: "1103", Username: "agent", Hostname: "vapi.example.com", AppID: testAppID}
	for _, plain := range []bool{true, false} {
		name := "json"
		if plain {
			name = "plain"
		}
		t.Run(name, func(t *testing.T) {
			wrap := &cobra.Command{
				Use: "wrap",
				RunE: func(cmd *cobra.Command, args []string) error {
					return emitCredential(cmd, cred, "generatedSecret123", true)
				},
			}
			root := testutil.NewTestRoot(wrap)
			args := []string{"wrap"}
			if plain {
				args = append(args, "--plain")
			}
			root.SetArgs(args)

			out := testutil.CaptureStdout(t, func() {
				if err := root.Execute(); err != nil {
					t.Fatalf("Execute() error = %v", err)
				}
			})

			// Still valid, complete JSON — ordering must not cost correctness.
			var got map[string]interface{}
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("output is not JSON: %q (%v)", out, err)
			}
			if got["password"] != "generatedSecret123" {
				t.Errorf("password = %v, want generatedSecret123", got["password"])
			}
			pwAt := strings.Index(out, `"password"`)
			if pwAt < 0 {
				t.Fatalf("output has no password field: %q", out)
			}
			for _, k := range []string{`"appId"`, `"hostname"`, `"id"`, `"passwordShownOnce"`, `"realmId"`, `"username"`} {
				at := strings.Index(out, k)
				if at < 0 {
					t.Errorf("missing field %s in %q", k, out)
					continue
				}
				if at < pwAt {
					t.Errorf("%s is written before \"password\" — a truncated write would lose the only copy of the secret: %q", k, out)
				}
			}
		})
	}
}

// TestPasswordFirstPayload_MarshalJSON covers the ordering type directly,
// including the caller-supplied case (no password key at all) and the fact that
// the remaining keys stay in a deterministic, sorted order.
func TestPasswordFirstPayload_MarshalJSON(t *testing.T) {
	withPassword, err := json.Marshal(passwordFirstPayload{
		"id": "1", "appId": "", "password": "s3cret", "passwordShownOnce": true,
	})
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if want := `{"password":"s3cret","appId":"","id":"1","passwordShownOnce":true}`; string(withPassword) != want {
		t.Errorf("MarshalJSON() = %s, want %s", withPassword, want)
	}
	withoutPassword, err := json.Marshal(passwordFirstPayload{"id": "1", "appId": ""})
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if want := `{"appId":"","id":"1"}`; string(withoutPassword) != want {
		t.Errorf("MarshalJSON() = %s, want %s", withoutPassword, want)
	}
}

// TestEmitCredential_RedactionStillRunsOnTheOrderedPath is the guard for the
// order-vs-redaction tradeoff. Ordering is achieved by wrapping the payload in a
// type output.RedactSecrets does not walk, so redaction has to run BEFORE the
// wrap. If that order is ever swapped, hash material would flow straight to
// stdout: this asserts the composition, not just the current payload's fields.
func TestEmitCredential_RedactionStillRunsOnTheOrderedPath(t *testing.T) {
	payload := map[string]interface{}{
		"id":       "1",
		"password": "s3cret",
		"Hash1":    "1be6abcaa8e9956021d30f33a3925b99",
		"hash1b":   "e028e6577a0bb1b90a33d30a110dbdfe",
	}
	redacted, ok := output.RedactSecrets(payload).(map[string]interface{})
	if !ok {
		t.Fatal("RedactSecrets did not return a map")
	}
	b, err := json.Marshal(passwordFirstPayload(redacted))
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if want := `{"password":"s3cret","id":"1"}`; string(b) != want {
		t.Errorf("redact-then-order = %s, want %s", b, want)
	}
	// The generated password is exempt from redaction by design — it is the
	// deliverable — and must survive both steps.
	if !strings.Contains(string(b), "s3cret") {
		t.Errorf("redact-then-order dropped the generated password: %s", b)
	}
}

// credentialCreateFaultStubServer rejects the POST with a structured Bandwidth
// fault at the given status: the server parsed the request and refused it, so
// no credential was created.
func credentialCreateFaultStubServer(t *testing.T, status int, code string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/realms/vapi"):
			w.Write([]byte(`<RealmResponse><Realm><Id>1103</Id>` +
				`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm><Status>ACTIVE</Status>` +
				`</Realm></RealmResponse>`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sipcredentials"):
			w.WriteHeader(status)
			w.Write([]byte(`<SipCredentialsResponse><Errors><Error>` +
				`<ErrorCode>` + code + `</ErrorCode><Description>rejected</Description>` +
				`</Error></Errors></SipCredentialsResponse>`))
		default:
			w.WriteHeader(404)
		}
	}))
}

// TestCredentialCreate_DefinitiveFaultDoesNotExitSecretUnavailable is the
// discrimination the generated-password branch was missing.
//
// The concrete failure: `credential create --generate-password` hits a 429.
// Nothing was created. The agent got exit 8 — "not retryable as-is, rotate the
// credential" — listed credentials, found none, and dead-ended. Exit 8 is only
// honest when the write MIGHT have landed; an *APIFault proves it did not,
// because the server parsed the request before refusing it.
func TestCredentialCreate_DefinitiveFaultDoesNotExitSecretUnavailable(t *testing.T) {
	cases := []struct {
		name   string
		status int
		code   string
		want   int
	}{
		{"rate limited", 429, "1001", cmdutil.ExitRateLimit},
		{"duplicate credential at live 400", 400, "23026", cmdutil.ExitConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := credentialCreateFaultStubServer(t, c.status, c.code)
			defer srv.Close()
			withStubService(t, srv)

			root := testutil.NewTestRoot(credentialCreateCmd)
			// Flag values are package vars bound to a shared cobra command, so every
			// flag this path reads is set explicitly — cobra only assigns on parse,
			// and a --password-stdin left true by an earlier test in this process
			// would trip the "exactly one password source" guard before the
			// branch under test is ever reached.
			root.SetArgs([]string{"create", "--realm", "vapi", "--username", "agent",
				"--generate-password", "--password-stdin=false", "--password-file=", "--if-not-exists=false", "--app-id="})

			err := root.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want the fault to surface")
			}
			var sue *cmdutil.SecretUnavailableError
			if errors.As(err, &sue) {
				t.Fatalf("error = %v, want NOT *cmdutil.SecretUnavailableError: a parsed rejection means nothing was created", err)
			}
			if got := cmdutil.ExitCodeForError(err); got != c.want {
				t.Errorf("ExitCodeForError() = %d, want %d; err = %v", got, c.want, err)
			}
		})
	}
}

// TestCredentialCreate_NonActiveRealmExitsConflict pins the client-side guard's
// exit code. The spec (line 140) assigns 4 to "credential on a non-ACTIVE
// realm"; it returned a bare fmt.Errorf and so exited 1, which an agent reads
// as an unexpected failure rather than "wait for the realm, then retry."
func TestCredentialCreate_NonActiveRealmExitsConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/realms/vapi") {
			w.Write([]byte(`<RealmResponse><Realm><Id>1103</Id>` +
				`<Realm>vapi-3efeaa.auth.bandwidth.com</Realm><Status>CREATE_PENDING</Status>` +
				`</Realm></RealmResponse>`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	withStubService(t, srv)

	root := testutil.NewTestRoot(credentialCreateCmd)
	root.SetArgs([]string{"create", "--realm", "vapi", "--username", "agent",
		"--generate-password", "--password-stdin=false", "--password-file=", "--if-not-exists=false", "--app-id="})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want a conflict for a non-ACTIVE realm")
	}
	if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitConflict {
		t.Errorf("ExitCodeForError() = %d, want ExitConflict (%d); err = %v", got, cmdutil.ExitConflict, err)
	}
	if !strings.Contains(err.Error(), "credentials can only be created on ACTIVE realms") {
		t.Errorf("error = %q, want the existing message preserved verbatim", err.Error())
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
