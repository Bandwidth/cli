package customerprofile

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	cpsvc "github.com/Bandwidth/cli/internal/customerprofile"
	"github.com/Bandwidth/cli/internal/testutil"
)

// Split into list and get so --plain output shape never depends on whether an
// optional argument was supplied: list is always an array, get always an object.
func TestHistoryListReturnsArray(t *testing.T) {
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"version":1},{"version":2}],"page":{"pageSize":50,"totalElements":2}}`))
	}, "history", "list", "abc", "--plain")
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("stdout = %q, want a JSON array", out)
	}
}

func TestHistoryGetReturnsObject(t *testing.T) {
	var gotPath string
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"data":{"version":2,"name":"Acme"}}`))
	}, "history", "get", "abc", "2", "--plain")
	if err != nil {
		t.Fatalf("history get: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("stdout = %q, want a JSON object", out)
	}
	if !strings.HasSuffix(gotPath, "/history/2") {
		t.Errorf("path = %q, want it to end in /history/2", gotPath)
	}
}

func TestHistoryGetRequiresBothArgs(t *testing.T) {
	if _, err := runCmd(t, nil, "history", "get", "abc"); err == nil {
		t.Fatal("expected an error when the version argument is missing")
	}
}

func TestHistoryListAllWalksPages(t *testing.T) {
	calls := 0
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"data":[{"version":1}],"page":{"pageSize":1,"totalElements":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"version":2}],"page":{"pageSize":1,"totalElements":2}}`))
	}, "history", "list", "abc", "--all", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("history list --all: %v", err)
	}
	if calls != 2 {
		t.Errorf("fetched %d pages, want 2", calls)
	}
	if !strings.Contains(out, `"version":1`) && !strings.Contains(out, `"version": 1`) {
		t.Errorf("stdout = %q, want items from the first page", out)
	}
	if !strings.Contains(out, `"version":2`) && !strings.Contains(out, `"version": 2`) {
		t.Errorf("stdout = %q, want items from the second page too", out)
	}
}

// TestHistoryListWarnsWhenTruncated guards against list.go's warnIfTruncated
// pattern silently not being mirrored here: a paginated, non---all history
// list must warn on stderr when more versions exist than the page returned,
// exactly like 'customer-profile list' does.
func TestHistoryListWarnsWhenTruncated(t *testing.T) {
	_, stderr, err := runCmdCapturingStderr(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"metadata":{"version":1}}],"page":{"pageSize":1,"totalElements":2}}`))
	}, "history", "list", "abc", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if !strings.Contains(stderr, "pass --all to fetch every page") {
		t.Errorf("stderr = %q, want a truncation warning", stderr)
	}
}

func TestHistoryListNoWarningWhenNotTruncated(t *testing.T) {
	_, stderr, err := runCmdCapturingStderr(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"metadata":{"version":1}},{"metadata":{"version":2}}],"page":{"pageSize":50,"totalElements":2}}`))
	}, "history", "list", "abc", "--plain")
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want no truncation warning", stderr)
	}
}

// runCmdCapturingStderr is runCmd's twin, but with testRoot's error writer
// pointed at a buffer instead of io.Discard so a test can assert on
// cmd.PrintErrf output. It is a separate function, rather than a change to
// runCmd's signature, because runCmd's (string, error) return is used by
// every other test in this package (including in other files this task
// must not modify) and changing it would ripple across the whole suite.
func runCmdCapturingStderr(t *testing.T, h http.HandlerFunc, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	resetFlags(testRoot)

	var srvURL string
	if h != nil {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		srvURL = srv.URL
	}

	orig := service
	service = func(cmd *cobra.Command) (*cpsvc.Service, error) {
		if srvURL == "" {
			t.Fatal("command made a request but no stub server was provided")
		}
		return cpsvc.NewService(api.NewClientNoAuth(srvURL), "9901287"), nil
	}
	t.Cleanup(func() { service = orig })

	testRoot.SetArgs(append([]string{Cmd.Name()}, args...))
	testRoot.SetOut(io.Discard)
	var errBuf bytes.Buffer
	testRoot.SetErr(&errBuf)
	t.Cleanup(func() { testRoot.SetErr(io.Discard) })

	out := testutil.CaptureStdout(t, func() { err = testRoot.Execute() })
	return out, errBuf.String(), err
}
