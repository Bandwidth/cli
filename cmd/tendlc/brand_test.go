package tendlc

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	tendlcsvc "github.com/Bandwidth/cli/internal/tendlc"
	"github.com/Bandwidth/cli/internal/testutil"
)

// testRoot is a single fake root, built once via testutil.NewTestRoot and
// reused by every runBrandCmd call in this package, with Cmd (the `tendlc`
// parent) as its only child. --format/--plain/--account-id/--environment
// live here, exactly as in production.
//
// It is deliberately NOT rebuilt per call — see resetFlags below and
// cmd/customerprofile/customerprofile_test.go's testRoot, which this mirrors:
// cobra caches a command's merged ancestor flags (parentsPflags) the first
// time it parses and never refreshes that cache for a different root object
// later, so a fresh-root-per-call harness would silently pin every
// package-level command (brandCmd, brandListCmd, ...) to the FIRST test's
// root.
var testRoot = testutil.NewTestRoot(Cmd)

// resetFlags restores every flag on cmd and all its descendants to its
// default value and clears the Changed bit, so state set by one test (e.g.
// --offset from a pagination test) cannot leak into the next.
func resetFlags(cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	for _, sub := range cmd.Commands() {
		resetFlags(sub)
	}
}

// newBrandStub starts an httptest.Server running handler and registers it to
// close on test cleanup.
func newBrandStub(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// stubBrandList answers any request with one brand on a single, non-truncated
// page. Good enough for tests that just need brand list to succeed.
func stubBrandList(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"bandwidthId":"WET8JUY8H0","brandId":"BGJR2BA"}],` +
			`"page":{"pageNumber":0,"pageSize":50,"totalElements":1,"totalPages":1}}`))
	})
}

// stubBrandListCapturing records the raw query string of every request to
// /brands so a test can assert on the deepObject filter encoding.
func stubBrandListCapturing(t *testing.T) (*httptest.Server, *[]string) {
	var queries []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"data":[],"page":{"pageNumber":0,"pageSize":50,"totalElements":0,"totalPages":0}}`))
	})
	return srv, &queries
}

// stubBrandListTruncated answers with a page that reports more records exist
// than were returned, so warnIfTruncated fires.
func stubBrandListTruncated(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"bandwidthId":"W1"}],` +
			`"page":{"pageNumber":0,"pageSize":1,"totalElements":5,"totalPages":5}}`))
	})
}

// stubBrandGetCapturing records the request path of every request so a test
// can assert the positional ID is passed through unchanged.
func stubBrandGetCapturing(t *testing.T) (*httptest.Server, *[]string) {
	var paths []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{"data":{"bandwidthId":"WET8JUY8H0","brandId":"BGJR2BA"}}`))
	})
	return srv, &paths
}

// stubBrandHistory answers /brands/.../history with one free-text entry.
func stubBrandHistory(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"createdDate":"2026-01-01T00:00:00Z",` +
			`"message":"Successfully updated brand"}],` +
			`"page":{"pageNumber":0,"pageSize":50,"totalElements":1,"totalPages":1}}`))
	})
}

// stubBrandListTwoPages serves genuinely different items on page one
// (offset 0) versus page two (offset 1), keyed off the request's offset
// query param — the same shape as
// cmd/customerprofile/customerprofile_test.go's TestListAllWalksEveryPage
// stub. totalElements=2 with pageSize=1 forces api.ForEachPage to fetch both
// pages under --all --limit 1.
func stubBrandListTwoPages(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		if offset == "" || offset == "0" {
			_, _ = w.Write([]byte(`{"data":[{"bandwidthId":"PAGE-A"}],` +
				`"page":{"pageNumber":0,"pageSize":1,"totalElements":2,"totalPages":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"bandwidthId":"PAGE-B"}],` +
			`"page":{"pageNumber":1,"pageSize":1,"totalElements":2,"totalPages":2}}`))
	})
}

// stubBrandHistoryTwoPages is stubBrandListTwoPages's twin for the history
// endpoint: distinct messages per page, keyed off the offset query param.
func stubBrandHistoryTwoPages(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		if offset == "" || offset == "0" {
			_, _ = w.Write([]byte(`{"data":[{"createdDate":"2026-01-02T00:00:00Z",` +
				`"message":"page one message"}],` +
				`"page":{"pageNumber":0,"pageSize":1,"totalElements":2,"totalPages":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"createdDate":"2026-01-01T00:00:00Z",` +
			`"message":"page two message"}],` +
			`"page":{"pageNumber":1,"pageSize":1,"totalElements":2,"totalPages":2}}`))
	})
}

// stubBrandErr answers every request with the given status code and body,
// for exercising roleGateError and other failure paths.
func stubBrandErr(t *testing.T, code int, body string) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	})
}

// runBrandCmd executes one `band tendlc <args...>` invocation against srv,
// resetting flag state and swapping the `service` seam beforehand. It
// returns stdout and stderr separately: stdout must stay parseable data,
// while truncation warnings and similar operator messages go to stderr via
// cmd.PrintErrf — see internal/output's direct os.Stdout write, which is why
// testutil.CaptureStdout is used here instead of cmd.SetOut.
func runBrandCmd(t *testing.T, srv *httptest.Server, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	resetFlags(testRoot)

	orig := service
	service = func(cmd *cobra.Command) (*tendlcsvc.Service, error) {
		if srv == nil {
			t.Fatal("command made a request but no stub server was provided")
		}
		return tendlcsvc.NewService(api.NewClientNoAuth(srv.URL), "9901287"), nil
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

func TestBrandListRejectsAllWithOffset(t *testing.T) {
	_, _, err := runBrandCmd(t, stubBrandList(t), "brand", "list", "--all", "--offset", "0")
	if err == nil {
		t.Fatal("want an error combining --all with --offset")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
}

func TestBrandListEncodesFiltersAsDeepObject(t *testing.T) {
	srv, queries := stubBrandListCapturing(t)
	if _, _, err := runBrandCmd(t, srv, "brand", "list",
		"--identity-status", "VERIFIED", "--company-name-contains", "Acme"); err != nil {
		t.Fatalf("brand list: %v", err)
	}
	q := (*queries)[0]
	// eq is silently ignored by the server on every field tested (see the
	// measurement recorded in brand_list.go), so every filter -- including
	// the enum-valued ones -- goes over the wire as contains, not eq.
	if !strings.Contains(q, "brandIdentityStatus%5Bcontains%5D=VERIFIED") {
		t.Errorf("query %q missing deepObject contains filter for brandIdentityStatus", q)
	}
	if !strings.Contains(q, "companyName%5Bcontains%5D=Acme") {
		t.Errorf("query %q missing deepObject contains filter", q)
	}
	if strings.Contains(q, "%5Beq%5D") {
		t.Errorf("query %q uses eq, which the API silently ignores", q)
	}
}

// TestBrandListAllFiltersSendContains covers the four flags that switched
// from eq to contains: brandId, customerProfileId, brandType, and
// brandIdentityStatus (identity status is exercised above). eq is accepted
// and silently dropped by the server on all of them -- see the measurement
// in brand_list.go -- so this locks in that none of them regress back to
// eq, which "looks" more correct but returns every brand on the account.
func TestBrandListAllFiltersSendContains(t *testing.T) {
	srv, queries := stubBrandListCapturing(t)
	if _, _, err := runBrandCmd(t, srv, "brand", "list",
		"--brand-id-contains", "BEXMPL1",
		"--customer-profile-id-contains", "9900000",
		"--brand-type", "PRIVATE_PROFIT"); err != nil {
		t.Fatalf("brand list: %v", err)
	}
	q := (*queries)[0]
	for _, want := range []string{
		"brandId%5Bcontains%5D=BEXMPL1",
		"customerProfileId%5Bcontains%5D=9900000",
		"brandType%5Bcontains%5D=PRIVATE_PROFIT",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query %q missing %q", q, want)
		}
	}
}

// stubBrandListIdentityStatusTrap serves three brands whose
// brandIdentityStatus values all contain the substring "VERIFIED":
// VERIFIED, VETTED_VERIFIED, and UNVERIFIED. It stands in for the
// production measurement in brand_list.go's RunE comment --
// brandIdentityStatus[contains]=VERIFIED matched all three on a real
// account -- so tests can assert on the CLI's response to that trap without
// a live account.
func stubBrandListIdentityStatusTrap(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"bandwidthId":"W1","brandId":"B1","brandIdentityStatus":"VERIFIED"},
			{"bandwidthId":"W2","brandId":"B2","brandIdentityStatus":"VETTED_VERIFIED"},
			{"bandwidthId":"W3","brandId":"B3","brandIdentityStatus":"UNVERIFIED"}
		],"page":{"pageNumber":0,"pageSize":50,"totalElements":3,"totalPages":1}}`))
	})
}

// TestBrandListIdentityStatusExcludesSubstringMatches proves the
// VERIFIED-matches-three-statuses trap is handled: the server's contains
// filter would return all three brands above (VERIFIED, VETTED_VERIFIED,
// and -- the dangerous one -- UNVERIFIED, the exact opposite of what was
// asked for), but the CLI must narrow that down to only the brand whose
// status is exactly "VERIFIED".
func TestBrandListIdentityStatusExcludesSubstringMatches(t *testing.T) {
	out, _, err := runBrandCmd(t, stubBrandListIdentityStatusTrap(t), "brand", "list",
		"--identity-status", "VERIFIED", "--plain")
	if err != nil {
		t.Fatalf("brand list: %v", err)
	}
	if !strings.Contains(out, `"B1"`) {
		t.Errorf("stdout = %q, want the exact VERIFIED brand B1", out)
	}
	if strings.Contains(out, `"B2"`) {
		t.Errorf("stdout = %q, must not include VETTED_VERIFIED brand B2 (contains-only match)", out)
	}
	if strings.Contains(out, `"B3"`) {
		t.Errorf("stdout = %q, must not include UNVERIFIED brand B3 -- the exact opposite of the requested status", out)
	}
}

func TestBrandListWarnsOnTruncationViaStderrOnly(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubBrandListTruncated(t), "brand", "list", "--limit", "1")
	if err != nil {
		t.Fatalf("brand list: %v", err)
	}
	if strings.Contains(out, "pass --all") {
		t.Error("truncation warning leaked into stdout; stdout must stay parseable")
	}
	if !strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr should carry the truncation warning, got %q", errOut)
	}
}

// TestBrandListAllWalksEveryPage exercises the ForEachPage accumulation
// branch, mirroring cmd/customerprofile/customerprofile_test.go's
// TestListAllWalksEveryPage: the stub serves distinct items per page, and the
// assertion requires BOTH pages' items in stdout, not just a count — an
// implementation that fetched page one twice (or dropped a page) would fail
// this even though len(all) might coincidentally match.
func TestBrandListAllWalksEveryPage(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubBrandListTwoPages(t), "brand", "list", "--all", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("brand list --all: %v", err)
	}
	if !strings.Contains(out, "PAGE-A") || !strings.Contains(out, "PAGE-B") {
		t.Errorf("stdout = %q, want items from both pages", out)
	}
	// --all walks every page, so nothing was left un-fetched. A truncation
	// warning here would be a lie: it would tell the caller records remain
	// when the command just fetched all of them.
	if strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr = %q, want no truncation warning when --all already walked every page", errOut)
	}
}

func TestBrandGetAcceptsEitherIdentifier(t *testing.T) {
	srv, paths := stubBrandGetCapturing(t)
	for _, id := range []string{"BGJR2BA", "WET8JUY8H0"} {
		if _, _, err := runBrandCmd(t, srv, "brand", "get", id); err != nil {
			t.Fatalf("brand get %s: %v", id, err)
		}
	}
	if len(*paths) != 2 || !strings.HasSuffix((*paths)[0], "/brands/BGJR2BA") || !strings.HasSuffix((*paths)[1], "/brands/WET8JUY8H0") {
		t.Errorf("paths = %v; get must pass the ID through unchanged", *paths)
	}
}

func TestBrandCommandsRejectStrayPositionals(t *testing.T) {
	// A stray positional on a read is harmless; on a write it is not, and the
	// guard belongs on every command so the rule is not a per-command judgment
	// call. PR 2 shipped a create without one and it silently created a
	// resource from a typo.
	cases := [][]string{
		{"brand", "list", "STRAY"},
		{"brand", "get"},
		{"brand", "get", "B1", "STRAY"},
		{"brand", "history"},
		{"brand", "history", "B1", "STRAY"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, _, err := runBrandCmd(t, stubBrandList(t), args...); err == nil {
				t.Fatal("want an argument error")
			}
		})
	}
}

func TestBrandHistoryReturnsMessageLog(t *testing.T) {
	out, _, err := runBrandCmd(t, stubBrandHistory(t), "brand", "history", "BGJR2BA")
	if err != nil {
		t.Fatalf("brand history: %v", err)
	}
	if !strings.Contains(out, "Successfully updated brand") {
		t.Errorf("stdout should carry history messages, got %q", out)
	}
}

// TestBrandHistoryAllWalksEveryPage is TestBrandListAllWalksEveryPage's twin
// for `brand history --all`.
func TestBrandHistoryAllWalksEveryPage(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubBrandHistoryTwoPages(t), "brand", "history", "BGJR2BA",
		"--all", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("brand history --all: %v", err)
	}
	if !strings.Contains(out, "page one message") || !strings.Contains(out, "page two message") {
		t.Errorf("stdout = %q, want messages from both pages", out)
	}
	if strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr = %q, want no truncation warning when --all already walked every page", errOut)
	}
}

func TestRoleGate403MapsToExitFour(t *testing.T) {
	_, _, err := runBrandCmd(t, stubBrandErr(t, 403,
		`{"errors":[{"description":"does not have access rights"}]}`), "brand", "list")
	if err == nil {
		t.Fatal("want an error on 403")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d — re-authenticating cannot add a role", code, cmdutil.ExitConflict)
	}
}

// requireConfirm has no caller yet in this task — brand list/get/history are
// all reads. It is added now because a later task (brand create/delete)
// needs it, and golangci-lint's unused check fails a helper with zero
// callers; this test is that caller until the write commands land.
func TestRequireConfirmGatesOnConfirmFlag(t *testing.T) {
	if err := requireConfirm(true, "should not fire"); err != nil {
		t.Fatalf("requireConfirm(true, ...) = %v, want nil", err)
	}
	err := requireConfirm(false, "this action needs --confirm because it deletes the brand")
	if err == nil {
		t.Fatal("requireConfirm(false, ...) = nil, want an error")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "deletes the brand") {
		t.Errorf("error = %q, want it to carry the specific consequence", err.Error())
	}
}
