package tendlc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	tendlcsvc "github.com/Bandwidth/cli/internal/tendlc"
	"github.com/Bandwidth/cli/internal/testutil"
)

// asyncTestCmd returns a bare command carrying the output flags awaitTerminal
// reads, plus a buffer capturing its stderr.
//
// stdout is NOT captured here: internal/output writes to os.Stdout directly,
// not to cmd.OutOrStdout(), so cmd.SetOut would silently capture nothing and
// every stdout assertion would pass against an empty string. Use
// testutil.CaptureStdout around the awaitTerminal call instead — the same
// mechanism cmd/customerprofile's runCmd uses.
//
// The command is its own root, which is what cmdutil.OutputFlags needs: it
// resolves --format and --plain through cmd.Root().
func asyncTestCmd() (*cobra.Command, *bytes.Buffer) {
	var errBuf bytes.Buffer
	c := &cobra.Command{Use: "x"}
	c.Flags().String("format", "json", "")
	c.Flags().Bool("plain", true, "")
	c.SetOut(io.Discard)
	c.SetErr(&errBuf)
	return c, &errBuf
}

// runAwait calls awaitTerminal with stdout captured.
func runAwait(t *testing.T, cmd *cobra.Command, tgt pollTarget, receipt map[string]any,
	timeout, interval time.Duration) (stdout string, err error) {
	t.Helper()
	stdout = testutil.CaptureStdout(t, func() {
		err = awaitTerminal(cmd, tgt, receipt, timeout, interval)
	})
	return stdout, err
}

func decodeReceipt(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &m); err != nil {
		t.Fatalf("stdout is not JSON (%v): %q", err, s)
	}
	return m
}

func TestAwaitTerminalSuccessPrintsFinalResource(t *testing.T) {
	cmd, _ := asyncTestCmd()
	target := pollTarget{
		Noun: "brand",
		Fetch: func() (map[string]any, bool, error) {
			return map[string]any{"bandwidthId": "WABC", "brandIdentityStatus": "VERIFIED"}, true, nil
		},
		Classify: func(o map[string]any) tendlcsvc.StateClass {
			s, _ := o["brandIdentityStatus"].(string)
			return tendlcsvc.ClassifyBrandIdentity(s)
		},
	}
	out, err := runAwait(t, cmd, target, map[string]any{"bandwidthId": "WABC"}, time.Second, time.Millisecond)
	if err != nil {
		t.Fatalf("want success, got %v", err)
	}
	got := decodeReceipt(t, out)
	if got["brandIdentityStatus"] != "VERIFIED" {
		t.Errorf("stdout should be the final resource, got %v", got)
	}
}

// A business failure is exit 4, and the resource still reaches stdout —
// the caller needs to see which state it settled in.
func TestAwaitTerminalBusinessFailureExitsFourWithResource(t *testing.T) {
	cmd, errBuf := asyncTestCmd()
	target := pollTarget{
		Noun: "brand",
		Fetch: func() (map[string]any, bool, error) {
			return map[string]any{"bandwidthId": "WABC", "brandIdentityStatus": "UNVERIFIED"}, true, nil
		},
		Classify: func(o map[string]any) tendlcsvc.StateClass {
			s, _ := o["brandIdentityStatus"].(string)
			return tendlcsvc.ClassifyBrandIdentity(s)
		},
		Remediate: func(o map[string]any) string {
			s, _ := o["brandIdentityStatus"].(string)
			return tendlcsvc.BrandRemediation(s)
		},
	}
	out, err := runAwait(t, cmd, target, map[string]any{"bandwidthId": "WABC"}, time.Second, time.Millisecond)
	if err == nil {
		t.Fatal("want an error for a business-failure state")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitConflict)
	}
	got := decodeReceipt(t, out)
	if got["bandwidthId"] != "WABC" {
		t.Errorf("stdout must still carry the resource, got %v", got)
	}
	if !strings.Contains(errBuf.String(), "reverify") {
		t.Errorf("stderr should carry state-specific remediation, got %q", errBuf.String())
	}
}

// The whole point of the runner: a timeout must not lose the ID.
func TestAwaitTerminalTimeoutStillEmitsReceipt(t *testing.T) {
	cmd, _ := asyncTestCmd()
	target := pollTarget{
		Noun: "brand",
		Fetch: func() (map[string]any, bool, error) {
			return map[string]any{"bandwidthId": "WABC", "brandIdentityStatus": "REGISTERING"}, true, nil
		},
		Classify: func(o map[string]any) tendlcsvc.StateClass {
			s, _ := o["brandIdentityStatus"].(string)
			return tendlcsvc.ClassifyBrandIdentity(s)
		},
	}
	receipt := map[string]any{"bandwidthId": "WABC", "resume": "band tendlc brand get WABC"}
	out, err := runAwait(t, cmd, target, receipt, 20*time.Millisecond, time.Millisecond)
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitTimeout {
		t.Errorf("exit code = %d, want %d (timeout)", code, cmdutil.ExitTimeout)
	}
	got := decodeReceipt(t, out)
	if got["bandwidthId"] != "WABC" {
		t.Fatalf("timeout must still print the ID, got %v", got)
	}
	if got["resume"] != "band tendlc brand get WABC" {
		t.Errorf("timeout receipt must carry a resume command, got %v", got)
	}
}

// A transport failure DURING polling is the nastiest case: the write was
// accepted, so exiting with only an error loses the ID forever.
func TestAwaitTerminalPollTransportErrorStillEmitsReceipt(t *testing.T) {
	cmd, _ := asyncTestCmd()
	boom := &api.APIError{StatusCode: 500, Body: "upstream exploded"}
	target := pollTarget{
		Noun:     "brand",
		Fetch:    func() (map[string]any, bool, error) { return nil, false, boom },
		Classify: func(map[string]any) tendlcsvc.StateClass { return tendlcsvc.StatePending },
	}
	out, err := runAwait(t, cmd, target, map[string]any{"bandwidthId": "WABC"}, time.Second, time.Millisecond)
	if err == nil {
		t.Fatal("want the poll error surfaced")
	}
	if !errors.Is(err, boom) && !strings.Contains(err.Error(), "upstream exploded") {
		t.Errorf("original error must be preserved, got %v", err)
	}
	got := decodeReceipt(t, out)
	if got["bandwidthId"] != "WABC" {
		t.Fatalf("a poll failure must still print the ID, got %v", got)
	}
}

// For create-style polls a 404 means "not readable yet", not "gone".
func TestAwaitTerminalNotFoundIsPendingForCreates(t *testing.T) {
	cmd, _ := asyncTestCmd()
	calls := 0
	target := pollTarget{
		Noun: "brand",
		Fetch: func() (map[string]any, bool, error) {
			calls++
			if calls < 3 {
				return nil, false, nil
			}
			return map[string]any{"bandwidthId": "WABC", "brandIdentityStatus": "VERIFIED"}, true, nil
		},
		Classify: func(o map[string]any) tendlcsvc.StateClass {
			s, _ := o["brandIdentityStatus"].(string)
			return tendlcsvc.ClassifyBrandIdentity(s)
		},
	}
	if _, err := runAwait(t, cmd, target, map[string]any{"bandwidthId": "WABC"}, time.Second, time.Millisecond); err != nil {
		t.Fatalf("404 before readiness must keep polling, got %v", err)
	}
	if calls < 3 {
		t.Errorf("polled %d times, want at least 3", calls)
	}
}

// For delete-style polls a 404 is the success condition. No command treats
// 404 as both, which is why this is a field on the target rather than a
// heuristic.
func TestAwaitTerminalGoneIsDoneForDeletes(t *testing.T) {
	cmd, _ := asyncTestCmd()
	target := pollTarget{
		Noun:       "brand",
		GoneIsDone: true,
		Fetch:      func() (map[string]any, bool, error) { return nil, false, nil },
		Classify:   func(map[string]any) tendlcsvc.StateClass { return tendlcsvc.StatePending },
	}
	receipt := map[string]any{"bandwidthId": "WABC", "deleted": true}
	out, err := runAwait(t, cmd, target, receipt, time.Second, time.Millisecond)
	if err != nil {
		t.Fatalf("want success when the resource is gone, got %v", err)
	}
	got := decodeReceipt(t, out)
	if got["deleted"] != true {
		t.Errorf("delete success should print the receipt, got %v", got)
	}
}

// fetchBrandStubServer returns a stub that answers GET .../brands/{id} with
// the given status code and body, and a Service pointed at it — the same
// api.NewClientNoAuth seam used by cmd/tendlc/status_test.go.
func fetchBrandStubServer(t *testing.T, code int, body string) *tendlcsvc.Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return tendlcsvc.NewService(api.NewClientNoAuth(srv.URL), "9901287")
}

// fetchBrand is the sole translator of a real 404 into pollTarget's
// found=false — the mechanism the create-vs-delete GoneIsDone contract
// depends on — so all three outcomes need direct coverage against a real
// HTTP response, not just the classifier logic above it.
func TestFetchBrandFoundReturnsObject(t *testing.T) {
	svc := fetchBrandStubServer(t, http.StatusOK, `{"data":{"bandwidthId":"WABC","brandIdentityStatus":"VERIFIED"}}`)
	obj, found, err := fetchBrand(svc, "WABC")()
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if !found {
		t.Fatal("want found=true for a 200 response")
	}
	if obj["bandwidthId"] != "WABC" {
		t.Errorf("obj = %v, want bandwidthId WABC", obj)
	}
}

// A 404 must translate to found=false with NO error — that translation is
// the whole point of fetchBrand. An error here would make a create poll fail
// immediately on a resource that just isn't readable yet, instead of
// retrying until it appears or the poll times out.
func TestFetchBrandNotFoundIsFoundFalseNotError(t *testing.T) {
	svc := fetchBrandStubServer(t, http.StatusNotFound, `{"errors":[{"description":"brand not found"}]}`)
	obj, found, err := fetchBrand(svc, "WABC")()
	if err != nil {
		t.Fatalf("want no error for a 404, got %v", err)
	}
	if found {
		t.Fatal("want found=false for a 404")
	}
	if obj != nil {
		t.Errorf("want a nil object for a 404, got %v", obj)
	}
}

func TestFetchBrandServerErrorIsError(t *testing.T) {
	svc := fetchBrandStubServer(t, http.StatusInternalServerError, `{"errors":[{"description":"boom"}]}`)
	_, found, err := fetchBrand(svc, "WABC")()
	if err == nil {
		t.Fatal("want an error for a 500")
	}
	if found {
		t.Error("want found=false alongside the error")
	}
}

// A 200 with a non-object data field (e.g. an array) is a malformed
// response, not "not ready yet". Reporting it as found=false would make a
// create poll spin silently until timeout instead of failing fast on a
// response shape it can never recover from.
func TestFetchBrandMalformedDataIsError(t *testing.T) {
	svc := fetchBrandStubServer(t, http.StatusOK, `{"data":[{"bandwidthId":"WABC"}]}`)
	_, found, err := fetchBrand(svc, "WABC")()
	if err == nil {
		t.Fatal("want an error when data is not an object")
	}
	if found {
		t.Error("want found=false alongside the error")
	}
}
