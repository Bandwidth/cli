package tendlc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// stubCampaignList answers any request with one campaign on a single,
// non-truncated page. Good enough for tests that just need campaign list to
// succeed.
func stubCampaignList(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"campaignId":"CEXMPL1","brandId":"BEXMPL1"}],` +
			`"page":{"pageNumber":0,"pageSize":50,"totalElements":1,"totalPages":1}}`))
	})
}

// stubCampaignListCapturing records the raw query string of every request to
// /campaigns so a test can assert on the deepObject filter encoding.
func stubCampaignListCapturing(t *testing.T) (*httptest.Server, *[]string) {
	var queries []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		_, _ = w.Write([]byte(`{"data":[],"page":{"pageNumber":0,"pageSize":50,"totalElements":0,"totalPages":0}}`))
	})
	return srv, &queries
}

// stubCampaignListTruncated answers with a page that reports more records
// exist than were returned, so warnIfTruncated fires.
func stubCampaignListTruncated(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"campaignId":"C1"}],` +
			`"page":{"pageNumber":0,"pageSize":1,"totalElements":5,"totalPages":5}}`))
	})
}

// stubCampaignListTwoPages serves genuinely different items on page one
// (offset 0) versus page two (offset 1), keyed off the request's offset
// query param. totalElements=2 with pageSize=1 forces api.ForEachPage to
// fetch both pages under --all --limit 1.
func stubCampaignListTwoPages(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		if offset == "" || offset == "0" {
			_, _ = w.Write([]byte(`{"data":[{"campaignId":"PAGE-A"}],` +
				`"page":{"pageNumber":0,"pageSize":1,"totalElements":2,"totalPages":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"campaignId":"PAGE-B"}],` +
			`"page":{"pageNumber":1,"pageSize":1,"totalElements":2,"totalPages":2}}`))
	})
}

// stubCampaignGetCapturing records the request path of every request so a
// test can assert the positional ID is passed through unchanged.
func stubCampaignGetCapturing(t *testing.T) (*httptest.Server, *[]string) {
	var paths []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{"data":{"campaignId":"CEXMPL1","brandId":"BEXMPL1"}}`))
	})
	return srv, &paths
}

// stubCampaignPhoneNumbers answers /campaigns/.../phoneNumbers with one
// phone number on a single, non-truncated page.
func stubCampaignPhoneNumbers(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"phoneNumber":"+15551234567"}],` +
			`"page":{"pageNumber":0,"pageSize":50,"totalElements":1,"totalPages":1}}`))
	})
}

// stubCampaignPhoneNumbersTruncated answers with a page that reports more
// numbers exist than were returned, so warnIfTruncated fires.
func stubCampaignPhoneNumbersTruncated(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"phoneNumber":"+15551234567"}],` +
			`"page":{"pageNumber":0,"pageSize":1,"totalElements":5,"totalPages":5}}`))
	})
}

// stubCampaignPhoneNumbersTwoPages is stubCampaignListTwoPages's twin for the
// phoneNumbers endpoint: distinct numbers per page, keyed off the offset
// query param.
func stubCampaignPhoneNumbersTwoPages(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		if offset == "" || offset == "0" {
			_, _ = w.Write([]byte(`{"data":[{"phoneNumber":"+15550000001"}],` +
				`"page":{"pageNumber":0,"pageSize":1,"totalElements":2,"totalPages":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"phoneNumber":"+15550000002"}],` +
			`"page":{"pageNumber":1,"pageSize":1,"totalElements":2,"totalPages":2}}`))
	})
}

// stubCampaignHistory answers /campaigns/.../history with one free-text
// entry.
func stubCampaignHistory(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"createdDate":"2026-01-01T00:00:00Z",` +
			`"message":"Successfully updated campaign"}],` +
			`"page":{"pageNumber":0,"pageSize":50,"totalElements":1,"totalPages":1}}`))
	})
}

// stubCampaignHistoryTwoPages is stubCampaignListTwoPages's twin for the
// history endpoint: distinct messages per page, keyed off the offset query
// param.
func stubCampaignHistoryTwoPages(t *testing.T) *httptest.Server {
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

func TestCampaignListRejectsAllWithOffset(t *testing.T) {
	_, _, err := runBrandCmd(t, stubCampaignList(t), "campaign", "list", "--all", "--offset", "0")
	if err == nil {
		t.Fatal("want an error combining --all with --offset")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
}

func TestCampaignListEncodesFiltersAsDeepObject(t *testing.T) {
	srv, queries := stubCampaignListCapturing(t)
	if _, _, err := runBrandCmd(t, srv, "campaign", "list",
		"--status", "REGISTERED", "--campaign-name-contains", "Acme",
		"--brand-id", "BEXMPL1", "--campaign-id", "CEXMPL1",
		"--usecase", "2FA", "--vetting-status", "VETTED_VERIFIED"); err != nil {
		t.Fatalf("campaign list: %v", err)
	}
	q := (*queries)[0]
	for _, want := range []string{
		"status%5Beq%5D=REGISTERED",
		"campaignName%5Bcontains%5D=Acme",
		"brandId%5Beq%5D=BEXMPL1",
		"campaignId%5Beq%5D=CEXMPL1",
		"usecase%5Beq%5D=2FA",
		"vettingStatus%5Beq%5D=VETTED_VERIFIED",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("query %q missing deepObject filter %q", q, want)
		}
	}
}

func TestCampaignListWarnsOnTruncationViaStderrOnly(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubCampaignListTruncated(t), "campaign", "list", "--limit", "1")
	if err != nil {
		t.Fatalf("campaign list: %v", err)
	}
	if strings.Contains(out, "pass --all") {
		t.Error("truncation warning leaked into stdout; stdout must stay parseable")
	}
	if !strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr should carry the truncation warning, got %q", errOut)
	}
}

// TestCampaignListAllWalksEveryPage exercises the ForEachPage accumulation
// branch: the stub serves distinct items per page, and the assertion
// requires BOTH pages' items in stdout, not just a count — an implementation
// that fetched page one twice (or dropped a page) would fail this even
// though len(all) might coincidentally match.
func TestCampaignListAllWalksEveryPage(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubCampaignListTwoPages(t), "campaign", "list", "--all", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("campaign list --all: %v", err)
	}
	if !strings.Contains(out, "PAGE-A") || !strings.Contains(out, "PAGE-B") {
		t.Errorf("stdout = %q, want items from both pages", out)
	}
	if strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr = %q, want no truncation warning when --all already walked every page", errOut)
	}
}

func TestCampaignGetPassesIDThrough(t *testing.T) {
	srv, paths := stubCampaignGetCapturing(t)
	if _, _, err := runBrandCmd(t, srv, "campaign", "get", "CEXMPL1"); err != nil {
		t.Fatalf("campaign get: %v", err)
	}
	if len(*paths) != 1 || !strings.HasSuffix((*paths)[0], "/campaigns/CEXMPL1") {
		t.Errorf("paths = %v; get must pass the ID through unchanged", *paths)
	}
}

func TestCampaignPhoneNumbersRejectsAllWithOffset(t *testing.T) {
	_, _, err := runBrandCmd(t, stubCampaignPhoneNumbers(t), "campaign", "numbers", "CEXMPL1", "--all", "--offset", "0")
	if err == nil {
		t.Fatal("want an error combining --all with --offset")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
}

func TestCampaignPhoneNumbersReturnsNumbers(t *testing.T) {
	out, _, err := runBrandCmd(t, stubCampaignPhoneNumbers(t), "campaign", "numbers", "CEXMPL1")
	if err != nil {
		t.Fatalf("campaign numbers: %v", err)
	}
	if !strings.Contains(out, "+15551234567") {
		t.Errorf("stdout should carry the phone number, got %q", out)
	}
}

func TestCampaignPhoneNumbersWarnsOnTruncationViaStderrOnly(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubCampaignPhoneNumbersTruncated(t), "campaign", "numbers", "CEXMPL1", "--limit", "1")
	if err != nil {
		t.Fatalf("campaign numbers: %v", err)
	}
	if strings.Contains(out, "pass --all") {
		t.Error("truncation warning leaked into stdout; stdout must stay parseable")
	}
	if !strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr should carry the truncation warning, got %q", errOut)
	}
}

// TestCampaignPhoneNumbersAllWalksEveryPage is
// TestCampaignListAllWalksEveryPage's twin for `campaign numbers --all`.
func TestCampaignPhoneNumbersAllWalksEveryPage(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubCampaignPhoneNumbersTwoPages(t), "campaign", "numbers", "CEXMPL1",
		"--all", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("campaign numbers --all: %v", err)
	}
	if !strings.Contains(out, "+15550000001") || !strings.Contains(out, "+15550000002") {
		t.Errorf("stdout = %q, want numbers from both pages", out)
	}
	if strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr = %q, want no truncation warning when --all already walked every page", errOut)
	}
}

func TestCampaignHistoryReturnsMessageLog(t *testing.T) {
	out, _, err := runBrandCmd(t, stubCampaignHistory(t), "campaign", "history", "CEXMPL1")
	if err != nil {
		t.Fatalf("campaign history: %v", err)
	}
	if !strings.Contains(out, "Successfully updated campaign") {
		t.Errorf("stdout should carry history messages, got %q", out)
	}
}

// TestCampaignHistoryAllWalksEveryPage is TestCampaignListAllWalksEveryPage's
// twin for `campaign history --all`.
func TestCampaignHistoryAllWalksEveryPage(t *testing.T) {
	out, errOut, err := runBrandCmd(t, stubCampaignHistoryTwoPages(t), "campaign", "history", "CEXMPL1",
		"--all", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("campaign history --all: %v", err)
	}
	if !strings.Contains(out, "page one message") || !strings.Contains(out, "page two message") {
		t.Errorf("stdout = %q, want messages from both pages", out)
	}
	if strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr = %q, want no truncation warning when --all already walked every page", errOut)
	}
}

func TestCampaignCommandsRejectStrayPositionals(t *testing.T) {
	// A stray positional on a read is harmless; on a write it is not, and the
	// guard belongs on every command so the rule is not a per-command
	// judgment call.
	cases := [][]string{
		{"campaign", "list", "STRAY"},
		{"campaign", "get"},
		{"campaign", "get", "C1", "STRAY"},
		{"campaign", "numbers"},
		{"campaign", "numbers", "C1", "STRAY"},
		{"campaign", "history"},
		{"campaign", "history", "C1", "STRAY"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, _, err := runBrandCmd(t, stubCampaignList(t), args...); err == nil {
				t.Fatal("want an argument error")
			}
		})
	}
}

func TestCampaignRoleGate403MapsToExitFour(t *testing.T) {
	_, _, err := runBrandCmd(t, stubBrandErr(t, 403,
		`{"errors":[{"description":"does not have access rights"}]}`), "campaign", "list")
	if err == nil {
		t.Fatal("want an error on 403")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d — re-authenticating cannot add a role", code, cmdutil.ExitConflict)
	}
}
