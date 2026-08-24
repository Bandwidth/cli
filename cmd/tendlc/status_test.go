package tendlc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	tendlcsvc "github.com/Bandwidth/cli/internal/tendlc"
	"github.com/Bandwidth/cli/internal/testutil"
)

// withStubStatusService swaps the package-level `service` var for the
// duration of a test so `band tendlc status` hits a stub server instead of
// performing real auth — the same seam pattern cmd/sip's `service` var uses.
func withStubStatusService(t *testing.T, baseURL string) {
	t.Helper()
	orig := service
	t.Cleanup(func() { service = orig })
	service = func(cmd *cobra.Command) (*tendlcsvc.Service, error) {
		return tendlcsvc.NewService(api.NewClientNoAuth(baseURL), "9901287"), nil
	}
}

// statusStubServerWithCode returns a stub that answers GET .../brands (the
// cheap probe `band tendlc status` issues via ListBrands) with the given
// status code and body.
func statusStubServerWithCode(t *testing.T, code int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/brands") {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
}

// runStatus executes `band tendlc status --plain` against whatever `service`
// currently resolves to, returning stdout and the command's error together —
// the pairing that matters for this command's exit-code contract.
func runStatus(t *testing.T) (string, error) {
	t.Helper()
	root := testutil.NewTestRoot(statusCmd)
	root.SetArgs([]string{"status", "--plain"})

	var err error
	out := testutil.CaptureStdout(t, func() {
		err = root.Execute()
	})
	return out, err
}

func decodeStatusOutput(t *testing.T, out string) map[string]interface{} {
	t.Helper()
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %q (%v)", out, err)
	}
	return got
}

func assertModeUnknown(t *testing.T, got map[string]interface{}) {
	t.Helper()
	mode, ok := got["mode"].(map[string]interface{})
	if !ok {
		t.Fatalf("mode missing or wrong type: %#v", got["mode"])
	}
	if mode["status"] != "unknown" || mode["reason"] != "not_discoverable" {
		t.Errorf("mode = %v, want unknown/not_discoverable", mode)
	}
}

// TestTendlcStatus_ProbeSucceeded covers the 200 path end to end: RunE must
// return nil (exit 0) and stdout must carry access=available/probe_succeeded.
func TestTendlcStatus_ProbeSucceeded(t *testing.T) {
	srv := statusStubServerWithCode(t, 200, `{"data":[]}`)
	defer srv.Close()
	withStubStatusService(t, srv.URL)

	out, err := runStatus(t)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}

	got := decodeStatusOutput(t, out)
	if got["access"] != "available" {
		t.Errorf("access = %v, want %q", got["access"], "available")
	}
	if got["reason"] != "probe_succeeded" {
		t.Errorf("reason = %v, want %q", got["reason"], "probe_succeeded")
	}
	assertModeUnknown(t, got)
}

// TestTendlcStatus_RecognizedForbidden covers a documented 403: this is the
// rule most at risk of regression — a successful probe reporting a negative
// fact must still exit 0, not surface as a command failure.
func TestTendlcStatus_RecognizedForbidden(t *testing.T) {
	srv := statusStubServerWithCode(t, 403,
		`{"errors":[{"description":"client does not have access rights to the content"}]}`)
	defer srv.Close()
	withStubStatusService(t, srv.URL)

	out, err := runStatus(t)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (a 403 probe result is not a command failure)", err)
	}

	got := decodeStatusOutput(t, out)
	if got["access"] != "unavailable" {
		t.Errorf("access = %v, want %q", got["access"], "unavailable")
	}
	if got["reason"] != "role_absent" {
		t.Errorf("reason = %v, want %q", got["reason"], "role_absent")
	}
	assertModeUnknown(t, got)
}

// TestTendlcStatus_UnrecognizedForbidden covers a 403 body that doesn't match
// any of the three documented substrings: still a successful probe (exit 0),
// mapped to the access_denied fallback reason.
func TestTendlcStatus_UnrecognizedForbidden(t *testing.T) {
	srv := statusStubServerWithCode(t, 403, `{"errors":[{"description":"something new"}]}`)
	defer srv.Close()
	withStubStatusService(t, srv.URL)

	out, err := runStatus(t)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil (a 403 probe result is not a command failure)", err)
	}

	got := decodeStatusOutput(t, out)
	if got["access"] != "unavailable" {
		t.Errorf("access = %v, want %q", got["access"], "unavailable")
	}
	if got["reason"] != "access_denied" {
		t.Errorf("reason = %v, want %q", got["reason"], "access_denied")
	}
	assertModeUnknown(t, got)
}

// TestTendlcStatus_ServerError covers the probe_failed path for a 5xx: the
// probe's job IS to report Registration Center availability, so even though
// this exits non-zero, an agent branching on stable JSON fields still needs
// unknown/probe_failed on stdout rather than an empty body.
func TestTendlcStatus_ServerError(t *testing.T) {
	srv := statusStubServerWithCode(t, 503, `{}`)
	defer srv.Close()
	withStubStatusService(t, srv.URL)

	out, err := runStatus(t)
	if err == nil {
		t.Fatal("Execute() error = nil, want a non-nil error for HTTP 503")
	}

	got := decodeStatusOutput(t, out)
	if got["access"] != "unknown" {
		t.Errorf("access = %v, want %q", got["access"], "unknown")
	}
	if got["reason"] != "probe_failed" {
		t.Errorf("reason = %v, want %q", got["reason"], "probe_failed")
	}
	assertModeUnknown(t, got)
}

// TestTendlcStatus_MalformedSuccessBody locks in the documented coupling
// between probe success and envelope decoding: svc.ListBrands parses the
// response body before returning, so a 200 whose body is not valid JSON
// cannot be distinguished from a genuine failure. The command must not
// report available/probe_succeeded off a body it could not read — it reports
// unknown/probe_failed and returns a non-nil error, same as any other
// failure to answer.
func TestTendlcStatus_MalformedSuccessBody(t *testing.T) {
	srv := statusStubServerWithCode(t, 200, `not json at all`)
	defer srv.Close()
	withStubStatusService(t, srv.URL)

	out, err := runStatus(t)
	if err == nil {
		t.Fatal("Execute() error = nil, want a non-nil error for an undecodable 200 body")
	}

	got := decodeStatusOutput(t, out)
	if got["access"] != "unknown" {
		t.Errorf("access = %v, want %q", got["access"], "unknown")
	}
	if got["reason"] != "probe_failed" {
		t.Errorf("reason = %v, want %q", got["reason"], "probe_failed")
	}
	assertModeUnknown(t, got)
}

// TestTendlcStatus_RejectsStrayPositional guards status.go's Args: cobra.NoArgs
// — before this, statusCmd was the only command in the tree with no Args
// guard at all, so `band tendlc status whatever` silently ignored the extra
// token and exited 0 instead of failing on the unrecognized argument. No
// stub service is installed: this must fail argument validation before ever
// calling `service`.
func TestTendlcStatus_RejectsStrayPositional(t *testing.T) {
	root := testutil.NewTestRoot(statusCmd)
	root.SetArgs([]string{"status", "whatever"})

	var err error
	testutil.CaptureStdout(t, func() {
		err = root.Execute()
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want a non-nil error for a stray positional")
	}
}

// TestTendlcStatus_TransportFailure is the regression lock for the bug where
// a bare transport error (never wrapped in *api.APIError) produced EMPTY
// stdout: RunE fell straight through to roleGateError without emitting
// anything. Closing the server before the request forces a connection
// refused, which api.Client wraps as a plain error, not *api.APIError.
func TestTendlcStatus_TransportFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // closed before use: any request now fails at the transport layer
	withStubStatusService(t, srv.URL)

	out, err := runStatus(t)
	if err == nil {
		t.Fatal("Execute() error = nil, want a non-nil error for a transport failure")
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("stdout is empty; want a stable unknown/probe_failed JSON document even on transport failure")
	}

	got := decodeStatusOutput(t, out)
	if got["access"] != "unknown" {
		t.Errorf("access = %v, want %q", got["access"], "unknown")
	}
	if got["reason"] != "probe_failed" {
		t.Errorf("reason = %v, want %q", got["reason"], "probe_failed")
	}
	assertModeUnknown(t, got)
}
