package tendlc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// stubNumberList answers any request with one phone number on a single,
// non-truncated page. Good enough for tests that just need number list to
// succeed.
func stubNumberList(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"phoneNumber":"+15555550100","status":"SUCCESS"}],` +
			`"page":{"pageNumber":0,"pageSize":50,"totalElements":1,"totalPages":1}}`))
	})
}

// stubNumberListCapturing records the raw query string of every request to
// /phoneNumbers so a test can assert on the deepObject filter encoding.
func stubNumberListCapturing(t *testing.T) (*httptest.Server, *[]string) {
	var queries []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"data":[],"page":{"pageNumber":0,"pageSize":50,"totalElements":0,"totalPages":0}}`))
	})
	return srv, &queries
}

// stubNumberListTwoPagesCapturing is stubNumberListTwoPages's twin that also
// records the raw query string of every request, so a test can assert a
// filter survives every page of an --all walk, not just the first request.
func stubNumberListTwoPagesCapturing(t *testing.T) (*httptest.Server, *[]string) {
	var queries []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		offset := r.URL.Query().Get("offset")
		if offset == "" || offset == "0" {
			_, _ = w.Write([]byte(`{"data":[{"phoneNumber":"+15555550100"}],` +
				`"page":{"pageNumber":0,"pageSize":1,"totalElements":2,"totalPages":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"phoneNumber":"+15555550199"}],` +
			`"page":{"pageNumber":1,"pageSize":1,"totalElements":2,"totalPages":2}}`))
	})
	return srv, &queries
}

// stubNumberListTruncated answers with a page that reports more records
// exist than were returned, so warnIfTruncated fires.
func stubNumberListTruncated(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"phoneNumber":"+15555550100"}],` +
			`"page":{"pageNumber":0,"pageSize":1,"totalElements":5,"totalPages":5}}`))
	})
}

// stubNumberListTwoPages serves genuinely different items on page one
// (offset 0) versus page two (offset 1), keyed off the request's offset
// query param. totalElements=2 with pageSize=1 forces api.ForEachPage to
// fetch both pages under --all --limit 1.
func stubNumberListTwoPages(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		if offset == "" || offset == "0" {
			_, _ = w.Write([]byte(`{"data":[{"phoneNumber":"+15555550100"}],` +
				`"page":{"pageNumber":0,"pageSize":1,"totalElements":2,"totalPages":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"phoneNumber":"+15555550199"}],` +
			`"page":{"pageNumber":1,"pageSize":1,"totalElements":2,"totalPages":2}}`))
	})
}

// stubNumberGetCapturing records the request path of every request so a test
// can assert the positional phone number is passed through unchanged.
func stubNumberGetCapturing(t *testing.T) (*httptest.Server, *[]string) {
	var paths []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{"data":{"phoneNumber":"+15555550100","status":"SUCCESS"}}`))
	})
	return srv, &paths
}

// stubNumberHistory answers /phoneNumbers/.../history with one free-text
// entry.
func stubNumberHistory(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"createdDate":"2026-01-01T00:00:00Z",` +
			`"message":"Successfully registered phone number"}],` +
			`"page":{"pageNumber":0,"pageSize":50,"totalElements":1,"totalPages":1}}`))
	})
}

// stubNumberHistoryTwoPages is stubNumberListTwoPages's twin for the history
// endpoint: distinct messages per page, keyed off the offset query param.
func stubNumberHistoryTwoPages(t *testing.T) *httptest.Server {
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

// TestNumberListHasNoStatusFilterFlag locks in the deliberate absence of
// --status (see numberListCmd's Long): status[eq] and status[contains] are
// both accepted and silently ignored by the server for every value tried,
// including one matching nothing, and every one of them returns every phone
// number on the account regardless. Unlike campaignId, there is no operator
// under which status genuinely filters, so nobody should reintroduce this
// flag without deleting this test and the reasoning it guards.
func TestNumberListHasNoStatusFilterFlag(t *testing.T) {
	if f := numberListCmd.Flags().Lookup("status"); f != nil {
		t.Errorf("number list must not have a --status flag, found %v", f)
	}

	// An exact-match --campaign-id (distinct from --campaign-id-contains,
	// which does and should exist) is deliberately absent for the same
	// reason: campaignId[eq] is silently ignored by the API and returns
	// every record regardless of value, so an exact-match flag would
	// reintroduce the filter-returns-everything anti-pattern this series
	// has now hit three times -- brand list --bandwidth-id, campaign list
	// --usecase, and this command's own --status above.
	if f := numberListCmd.Flags().Lookup("campaign-id"); f != nil {
		t.Errorf("number list must not have an exact-match --campaign-id flag, found %v", f)
	}
	if f := numberListCmd.Flags().Lookup("campaign-id-contains"); f == nil {
		t.Error("number list must have --campaign-id-contains, the flag that genuinely filters")
	}
}

// TestNumberListCampaignIDContainsSendsContains covers the one filter that
// genuinely works on this endpoint: campaignId[contains]. campaignId[eq] is
// accepted and silently ignored by the server (returns every number
// regardless of value) exactly like status -- see numberListCmd's Long --
// so this locks in that the flag never regresses back to eq, which "looks"
// more correct for an ID filter but would return every number on the
// account.
func TestNumberListCampaignIDContainsSendsContains(t *testing.T) {
	srv, queries := stubNumberListCapturing(t)
	if _, _, err := runBrandCmd(t, srv, "number", "list", "--campaign-id-contains", "CEXMPL1"); err != nil {
		t.Fatalf("number list: %v", err)
	}
	q := (*queries)[0]
	if !strings.Contains(q, "campaignId%5Bcontains%5D=CEXMPL1") {
		t.Errorf("query %q missing deepObject contains filter for campaignId", q)
	}
	if strings.Contains(q, "campaignId%5Beq%5D") {
		t.Errorf("query %q uses eq for campaignId, which the API silently ignores", q)
	}
}

// TestNumberListCampaignIDContainsSurvivesAllPagination confirms the filter
// is threaded through every page of an --all walk, not just the first
// request -- an implementation that captured filters once for the first
// call and then paginated with a bare ForEachPage closure ignoring them
// would pass a single-page filter test but silently drop the filter from
// page two onward.
func TestNumberListCampaignIDContainsSurvivesAllPagination(t *testing.T) {
	srv, queries := stubNumberListTwoPagesCapturing(t)
	if _, _, err := runBrandCmd(t, srv, "number", "list",
		"--campaign-id-contains", "CEXMPL1", "--all", "--limit", "1"); err != nil {
		t.Fatalf("number list --all: %v", err)
	}
	if len(*queries) < 2 {
		t.Fatalf("got %d requests, want at least 2 (one per page)", len(*queries))
	}
	for i, q := range *queries {
		if !strings.Contains(q, "campaignId%5Bcontains%5D=CEXMPL1") {
			t.Errorf("page %d query %q missing the campaignId filter", i, q)
		}
	}
}

func TestNumberListRejectsAllWithOffset(t *testing.T) {
	_, _, err := runBrandCmd(t, stubNumberList(t), "number", "list", "--all", "--offset", "0")
	if err == nil {
		t.Fatal("want an error combining --all with --offset")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
}

func TestNumberListReturnsNumbers(t *testing.T) {
	out, _, err := runBrandCmd(t, stubNumberList(t), "number", "list")
	if err != nil {
		t.Fatalf("number list: %v", err)
	}
	if !strings.Contains(out, "+15555550100") {
		t.Errorf("stdout should carry the phone number, got %q", out)
	}
}

func TestNumberListWarnsOnTruncationViaStderrOnly(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubNumberListTruncated(t), "number", "list", "--limit", "1")
	if err != nil {
		t.Fatalf("number list: %v", err)
	}
	if strings.Contains(out, "pass --all") {
		t.Error("truncation warning leaked into stdout; stdout must stay parseable")
	}
	if !strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr should carry the truncation warning, got %q", errOut)
	}
}

// TestNumberListAllWalksEveryPage exercises the ForEachPage accumulation
// branch: the stub serves distinct items per page, and the assertion
// requires BOTH pages' items in stdout, not just a count -- an
// implementation that fetched page one twice (or dropped a page) would fail
// this even though len(all) might coincidentally match.
func TestNumberListAllWalksEveryPage(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubNumberListTwoPages(t), "number", "list", "--all", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("number list --all: %v", err)
	}
	if !strings.Contains(out, "+15555550100") || !strings.Contains(out, "+15555550199") {
		t.Errorf("stdout = %q, want numbers from both pages", out)
	}
	if strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr = %q, want no truncation warning when --all already walked every page", errOut)
	}
}

func TestNumberGetPassesPhoneNumberThrough(t *testing.T) {
	srv, paths := stubNumberGetCapturing(t)
	if _, _, err := runBrandCmd(t, srv, "number", "get", "+15555550100"); err != nil {
		t.Fatalf("number get: %v", err)
	}
	if len(*paths) != 1 || !strings.HasSuffix((*paths)[0], "/phoneNumbers/+15555550100") {
		t.Errorf("paths = %v; get must pass the phone number through unchanged", *paths)
	}
}

// TestNumberGetNotFoundMapsToExitThree covers the "ship it plainly" decision
// in numberDetailCmd's Long: no bespoke handling for a 404, just the normal
// error path, which must still land on exit 3.
func TestNumberGetNotFoundMapsToExitThree(t *testing.T) {
	_, _, err := runBrandCmd(t, stubBrandErr(t, 404, `{"errors":[{"description":"not found"}]}`),
		"number", "get", "+15555550100")
	if err == nil {
		t.Fatal("want an error on 404")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitNotFound {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitNotFound)
	}
}

func TestNumberHistoryRejectsAllWithOffset(t *testing.T) {
	_, _, err := runBrandCmd(t, stubNumberHistory(t), "number", "history", "+15555550100", "--all", "--offset", "0")
	if err == nil {
		t.Fatal("want an error combining --all with --offset")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
}

func TestNumberHistoryReturnsMessageLog(t *testing.T) {
	out, _, err := runBrandCmd(t, stubNumberHistory(t), "number", "history", "+15555550100")
	if err != nil {
		t.Fatalf("number history: %v", err)
	}
	if !strings.Contains(out, "Successfully registered phone number") {
		t.Errorf("stdout should carry history messages, got %q", out)
	}
}

func TestNumberHistoryWarnsOnTruncationViaStderrOnly(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubNumberListTruncated(t), "number", "history", "+15555550100", "--limit", "1")
	if err != nil {
		t.Fatalf("number history: %v", err)
	}
	if strings.Contains(out, "pass --all") {
		t.Error("truncation warning leaked into stdout; stdout must stay parseable")
	}
	if !strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr should carry the truncation warning, got %q", errOut)
	}
}

// TestNumberHistoryAllWalksEveryPage is TestNumberListAllWalksEveryPage's
// twin for `number history --all`.
func TestNumberHistoryAllWalksEveryPage(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubNumberHistoryTwoPages(t), "number", "history", "+15555550100",
		"--all", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("number history --all: %v", err)
	}
	if !strings.Contains(out, "page one message") || !strings.Contains(out, "page two message") {
		t.Errorf("stdout = %q, want messages from both pages", out)
	}
	if strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr = %q, want no truncation warning when --all already walked every page", errOut)
	}
}

func TestNumberCommandsRejectStrayPositionals(t *testing.T) {
	// A stray positional on a read is harmless; on a write it is not, and the
	// guard belongs on every command so the rule is not a per-command
	// judgment call.
	cases := [][]string{
		{"number", "list", "STRAY"},
		{"number", "get"},
		{"number", "get", "+15555550100", "STRAY"},
		{"number", "history"},
		{"number", "history", "+15555550100", "STRAY"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, _, err := runBrandCmd(t, stubNumberList(t), args...); err == nil {
				t.Fatal("want an argument error")
			}
		})
	}
}

func TestNumberRoleGate403MapsToExitFour(t *testing.T) {
	_, _, err := runBrandCmd(t, stubBrandErr(t, 403,
		`{"errors":[{"description":"does not have access rights"}]}`), "number", "list")
	if err == nil {
		t.Fatal("want an error on 403")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d — re-authenticating cannot add a role", code, cmdutil.ExitConflict)
	}
}

// TestNumberCommandTreeHasNoLegacyFlatGet guards Task 3's removal of the
// legacy flat `band tendlc number <tn>` command: numberCmd (Use: "number")
// is now a plain parent, declared the same way as brandCmd and campaignCmd,
// with no RunE of its own. A bare `number <tn>` therefore resolves to
// numberCmd itself with an unconsumed positional, not to a get-style
// command — there is no more shorthand for `number get <tn>`. This also
// guards against a regression back to the pre-Task-3 collision risk: if a
// future change ever adds a second sibling command also named "number",
// this test's child-count assertion below would catch it, since cobra's
// Find matches children by name and returns the first match with no
// tie-break.
func TestNumberCommandTreeHasNoLegacyFlatGet(t *testing.T) {
	found, _, err := Cmd.Find([]string{"number", "+15555550100"})
	if err != nil {
		t.Fatalf("Find(number, <tn>): %v", err)
	}
	if found != numberCmd {
		t.Errorf("bare `number <tn>` resolved to %q, want it to fall through to the numberCmd parent (no legacy get shorthand)", found.CommandPath())
	}

	found, _, err = Cmd.Find([]string{"number", "list"})
	if err != nil {
		t.Fatalf("Find(number, list): %v", err)
	}
	if found != numberListCmd {
		t.Errorf("`number list` resolved to %q, want numberListCmd", found.CommandPath())
	}

	found, _, err = Cmd.Find([]string{"number", "get", "+15555550100"})
	if err != nil {
		t.Fatalf("Find(number, get, <tn>): %v", err)
	}
	if found != numberDetailCmd {
		t.Errorf("`number get <tn>` resolved to %q, want numberDetailCmd", found.CommandPath())
	}

	found, _, err = Cmd.Find([]string{"number", "history", "+15555550100"})
	if err != nil {
		t.Fatalf("Find(number, history, <tn>): %v", err)
	}
	if found != numberHistoryCmd {
		t.Errorf("`number history <tn>` resolved to %q, want numberHistoryCmd", found.CommandPath())
	}

	count := 0
	for _, c := range Cmd.Commands() {
		if c.Name() == "number" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Cmd has %d children named %q, want exactly 1", count, "number")
	}
}
