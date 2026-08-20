package tendlc

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fmt"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// validCampaignCreateArgs returns a full, valid flag set for `campaign
// create` (including both HELP fields, so the advisory does not fire), so
// each test only needs to state what it adds, overrides, or omits.
func validCampaignCreateArgs(extra ...string) []string {
	args := []string{
		"campaign", "create",
		"--brand-id", "BEXMPL1",
		"--usecase", "ACCOUNT_NOTIFICATION",
		"--description", "Sends account notifications to opted-in subscribers about their account status.",
		"--sample1", "Your account balance is now available. Reply STOP to opt out.",
		"--message-flow", "Customer opts in via web form; campaign sends account notifications only.",
		"--help-message", "For help, reply HELP or contact support.",
		"--help-keywords", "HELP,INFO",
	}
	return append(args, extra...)
}

// validCampaignCreateArgsNoHelp is validCampaignCreateArgs without the two
// HELP fields, so the non-fatal help advisory fires.
func validCampaignCreateArgsNoHelp(extra ...string) []string {
	args := []string{
		"campaign", "create",
		"--brand-id", "BEXMPL1",
		"--usecase", "ACCOUNT_NOTIFICATION",
		"--description", "Sends account notifications to opted-in subscribers about their account status.",
		"--sample1", "Your account balance is now available. Reply STOP to opt out.",
		"--message-flow", "Customer opts in via web form; campaign sends account notifications only.",
	}
	return append(args, extra...)
}

// stubCampaignBrandThenCreateCapturing answers GET .../brands/{id} (the
// create pre-flight) with the given brandIdentityStatus, and POST
// .../campaigns with a 202 carrying bandwidthID, recording each POST's raw
// JSON body so a test can assert exactly what was sent.
func stubCampaignBrandThenCreateCapturing(t *testing.T, brandStatus, bandwidthID string) (*httptest.Server, *[]string) {
	var bodies []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/brands/") {
			_, _ = fmt.Fprintf(w, `{"data":{"bandwidthId":"BRANDBW1","brandId":"BEXMPL1","brandIdentityStatus":%q}}`, brandStatus)
			return
		}
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, `{"data":{"bandwidthId":%q}}`, bandwidthID)
	})
	return srv, &bodies
}

// stubCampaignPreflightServer answers GET .../brands/{id} with the given
// status code and body, and records the raw body of any POST it receives
// (which should not happen when the pre-flight blocks the create).
func stubCampaignPreflightServer(t *testing.T, brandCode int, brandBody string) (*httptest.Server, *[]string) {
	var bodies []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/brands/") {
			w.WriteHeader(brandCode)
			_, _ = w.Write([]byte(brandBody))
			return
		}
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"data":{"bandwidthId":"WSHOULD-NOT-EXIST"}}`))
	})
	return srv, &bodies
}

// stubCampaignCreateThenPoll answers the brand pre-flight with brandStatus,
// POST .../campaigns with a 202 accepting bandwidthID, and GET
// .../campaigns/{id} with campaignStatus on every poll — enough for a
// --wait test that settles on the very first poll, so it never has to sleep
// out the poll interval.
func stubCampaignCreateThenPoll(t *testing.T, brandStatus, bandwidthID, campaignStatus string) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"data":{"bandwidthId":%q}}`, bandwidthID)
		case strings.Contains(r.URL.Path, "/brands/"):
			_, _ = fmt.Fprintf(w, `{"data":{"bandwidthId":"BRANDBW1","brandId":"BEXMPL1","brandIdentityStatus":%q}}`, brandStatus)
		default: // GET .../campaigns/{id} poll
			_, _ = fmt.Fprintf(w, `{"data":{"bandwidthId":%q,"campaignId":"CEXMPL1","status":%q}}`, bandwidthID, campaignStatus)
		}
	})
}

// Test 1: every missing required flag is named in one exit-6 error, and
// nothing is ever sent — passing a nil server means the `service` seam
// t.Fatal's if RunE ever calls it, so this test cannot pass by accident even
// if RunE regressed to building the service (or worse, the pre-flight/create
// request) before validating.
func TestCampaignCreateMissingRequiredFlagsAggregate(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "campaign", "create")
	if err == nil {
		t.Fatal("want an error for missing required flags")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	for _, want := range []string{"--brand-id", "--usecase", "--description", "--sample1", "--message-flow"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, missing flag %q", err.Error(), want)
		}
	}
}

// Test 2: an invalid --usecase exits 6 and lists the valid values. Zero
// requests for the same reason as Test 1.
func TestCampaignCreateInvalidUsecaseListsValidValues(t *testing.T) {
	args := validCampaignCreateArgs("--usecase", "NOT_A_REAL_USECASE")
	_, _, err := runBrandCmd(t, nil, args...)
	if err == nil {
		t.Fatal("want an error for an invalid usecase")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "ACCOUNT_NOTIFICATION") {
		t.Errorf("error = %q, want it to list valid usecases", err.Error())
	}
}

// Test 3: a valid create — brand pre-flight VERIFIED — posts the built body
// and prints a receipt carrying bandwidthId.
func TestCampaignCreatePostsBuiltBodyAndPrintsReceipt(t *testing.T) {
	srv, bodies := stubCampaignBrandThenCreateCapturing(t, "VERIFIED", "CNEW1")

	out, _, err := runBrandCmd(t, srv, validCampaignCreateArgs()...)
	if err != nil {
		t.Fatalf("campaign create: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly one POST, got %d", len(*bodies))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if sent["brandId"] != "BEXMPL1" || sent["usecase"] != "ACCOUNT_NOTIFICATION" {
		t.Errorf("posted body = %v, missing built fields", sent)
	}

	got := decodeStdout(t, out)
	if got["bandwidthId"] != "CNEW1" {
		t.Errorf("stdout = %v, want bandwidthId CNEW1", got)
	}
	if got["status"] != "accepted" {
		t.Errorf("stdout = %v, want status accepted", got)
	}
}

// Test 4: --wait polls to REGISTERED and prints the final campaign resource.
func TestCampaignCreateWaitPollsToRegistered(t *testing.T) {
	srv := stubCampaignCreateThenPoll(t, "VERIFIED", "CNEW1", "REGISTERED")

	out, _, err := runBrandCmd(t, srv, validCampaignCreateArgs("--wait", "--timeout", "5")...)
	if err != nil {
		t.Fatalf("campaign create --wait: %v", err)
	}
	got := decodeStdout(t, out)
	if got["status"] != "REGISTERED" {
		t.Errorf("stdout = %v, want the final REGISTERED resource", got)
	}
	if got["bandwidthId"] != "CNEW1" {
		t.Errorf("stdout = %v, want bandwidthId CNEW1", got)
	}
}

// Test 5: --wait on a campaign that settles DECLINED exits 4 with
// remediation on stderr — and, load-bearing, that remediation must carry the
// REAL campaign ID, not the literal placeholder text "<campaign-id>" that
// CampaignRemediation embeds. This is the regression test for the
// substitution wiring in campaignCreateCmd's Remediate closure.
func TestCampaignCreateWaitDeclinedExitsConflictWithRealID(t *testing.T) {
	srv := stubCampaignCreateThenPoll(t, "VERIFIED", "CNEW1", "DECLINED")

	out, errOut, err := runBrandCmd(t, srv, validCampaignCreateArgs("--wait", "--timeout", "5")...)
	if err == nil {
		t.Fatal("want an error for a campaign that settles DECLINED")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitConflict)
	}
	got := decodeStdout(t, out)
	if got["status"] != "DECLINED" {
		t.Errorf("stdout = %v, want the DECLINED resource still printed", got)
	}
	if !strings.Contains(errOut, "nudge CNEW1") && !strings.Contains(errOut, "nudge") {
		t.Errorf("stderr should carry DECLINED remediation mentioning nudge, got %q", errOut)
	}
	if !strings.Contains(errOut, "CNEW1") {
		t.Errorf("stderr = %q, want the real campaign ID substituted in", errOut)
	}
	if strings.Contains(errOut, "<campaign-id>") {
		t.Errorf("stderr = %q, must not contain the literal placeholder text", errOut)
	}
}

// Test 6: a --wait timeout exits 5 and still emits a receipt carrying
// bandwidthId and the last-seen status — the whole point of awaitTerminal.
// --timeout 0 makes the deadline already past by the time the first poll
// returns, so this fails fast instead of waiting out the real poll interval.
func TestCampaignCreateWaitTimeoutCarriesIDAndLastSeenStatus(t *testing.T) {
	srv := stubCampaignCreateThenPoll(t, "VERIFIED", "CNEW1", "REGISTERING")

	out, _, err := runBrandCmd(t, srv, validCampaignCreateArgs("--wait", "--timeout", "0")...)
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitTimeout {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitTimeout)
	}
	got := decodeStdout(t, out)
	if got["bandwidthId"] != "CNEW1" {
		t.Fatalf("timeout must still print the ID, got %v", got)
	}
	if got["lastSeenStatus"] != "REGISTERING" {
		t.Errorf("timeout receipt must carry the last-seen status, got %v", got)
	}
}

// Test 7: a pre-flight 404 on the brand stops the create at exit 3 and makes
// NO request to /campaigns.
func TestCampaignCreatePreflight404BlocksCreate(t *testing.T) {
	srv, bodies := stubCampaignPreflightServer(t, http.StatusNotFound,
		`{"errors":[{"description":"brand not found"}]}`)

	_, _, err := runBrandCmd(t, srv, validCampaignCreateArgs()...)
	if err == nil {
		t.Fatal("want an error when the brand pre-flight 404s")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitNotFound {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitNotFound)
	}
	if !strings.Contains(err.Error(), "BEXMPL1") {
		t.Errorf("error = %q, want it to name the brand ID", err.Error())
	}
	if len(*bodies) != 0 {
		t.Errorf("want zero requests to /campaigns, got %d", len(*bodies))
	}
}

// Test 8: a pre-flight success on a brand that is NOT VERIFIED or
// VETTED_VERIFIED exits 4 naming the blocking identity status, and makes no
// request to /campaigns — a campaign requires a verified brand, so this
// fails fast rather than letting the API return an opaque error.
func TestCampaignCreatePreflightUnverifiedBrandExitsConflict(t *testing.T) {
	srv, bodies := stubCampaignPreflightServer(t, http.StatusOK,
		`{"data":{"bandwidthId":"BRANDBW1","brandId":"BEXMPL1","brandIdentityStatus":"REGISTERING"}}`)

	_, _, err := runBrandCmd(t, srv, validCampaignCreateArgs()...)
	if err == nil {
		t.Fatal("want an error for a brand that is not VERIFIED or VETTED_VERIFIED")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitConflict)
	}
	if !strings.Contains(err.Error(), "REGISTERING") {
		t.Errorf("error = %q, want it to name the blocking identity status", err.Error())
	}
	if len(*bodies) != 0 {
		t.Errorf("want zero requests to /campaigns, got %d", len(*bodies))
	}
}

// Test 9: a pre-flight 403 proceeds with the create anyway and warns on
// stderr — Campaign Management is the role this command needs, and a brand
// pre-flight check must not block a caller entitled to create campaigns just
// because they lack whatever role the pre-flight itself needs.
func TestCampaignCreatePreflight403ProceedsWithWarning(t *testing.T) {
	srv, bodies := stubCampaignPreflightServer(t, http.StatusForbidden,
		`{"errors":[{"description":"does not have access rights"}]}`)

	out, errOut, err := runBrandCmd(t, srv, validCampaignCreateArgs()...)
	if err != nil {
		t.Fatalf("campaign create: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want the create to proceed despite the 403 pre-flight, got %d requests", len(*bodies))
	}
	got := decodeStdout(t, out)
	if _, ok := got["bandwidthId"]; !ok {
		t.Errorf("stdout = %v, want the create receipt", got)
	}
	if !strings.Contains(errOut, "BEXMPL1") {
		t.Errorf("stderr = %q, want a warning naming the brand ID", errOut)
	}
}

// Test 10: a stray positional is rejected by cobra.NoArgs before RunE ever
// runs, so no request is made at all.
func TestCampaignCreateRejectsStrayPositional(t *testing.T) {
	args := append([]string{"campaign", "create", "STRAY"}, validCampaignCreateArgs()[2:]...)
	_, _, err := runBrandCmd(t, nil, args...)
	if err == nil {
		t.Fatal("want an argument error for a stray positional")
	}
}

// Test 11: the non-fatal help advisory appears on stderr, and must NOT leak
// into stdout — stdout is data only, and a warning mixed into a JSON receipt
// would corrupt it for a parsing caller.
func TestCampaignCreateHelpAdvisoryOnStderrNotStdout(t *testing.T) {
	srv, _ := stubCampaignBrandThenCreateCapturing(t, "VERIFIED", "CNEW1")

	out, errOut, err := runBrandCmd(t, srv, validCampaignCreateArgsNoHelp()...)
	if err != nil {
		t.Fatalf("campaign create: %v", err)
	}
	if !strings.Contains(errOut, "--help-message") || !strings.Contains(errOut, "--help-keywords") {
		t.Errorf("stderr = %q, want the help advisory naming both missing flags", errOut)
	}
	if strings.Contains(out, "help-message") || strings.Contains(out, "warning") {
		t.Errorf("stdout = %q, must not carry the help advisory", out)
	}
}

// Test 12: an explicitly-passed --age-gated=false reaches the posted body as
// false, not omitted — BuildCampaignCreateRequest keys booleans on the
// changed map, not on the zero value, precisely so this case works.
func TestCampaignCreateAgeGatedFalseReachesBody(t *testing.T) {
	srv, bodies := stubCampaignBrandThenCreateCapturing(t, "VERIFIED", "CNEW1")

	_, _, err := runBrandCmd(t, srv, validCampaignCreateArgs("--age-gated=false")...)
	if err != nil {
		t.Fatalf("campaign create: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly one POST, got %d", len(*bodies))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	v, present := sent["ageGated"]
	if !present {
		t.Fatalf("posted body = %v, want ageGated present", sent)
	}
	if v != false {
		t.Errorf("posted body ageGated = %v, want false", v)
	}
}

// Test 13: sync posts exactly {"campaignId": id}, plus campaignName only
// when passed, and offers no --wait flag — it writes against a campaign
// usually already in a terminal state, so polling would report success
// before the change applied.
func TestCampaignSyncPostsExactBody(t *testing.T) {
	srv, bodies := stubBrandCreateCapturing(t, "CEXMPL1")

	out, _, err := runBrandCmd(t, srv, "campaign", "sync", "CEXMPL1")
	if err != nil {
		t.Fatalf("campaign sync: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly one POST, got %d", len(*bodies))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if len(sent) != 1 || sent["campaignId"] != "CEXMPL1" {
		t.Errorf("posted body = %v, want exactly {\"campaignId\":\"CEXMPL1\"}", sent)
	}
	got := decodeStdout(t, out)
	if got["bandwidthId"] != "CEXMPL1" {
		t.Errorf("stdout = %v, want the sync receipt", got)
	}
}

// Test 13b: sync with --campaign-name adds exactly that one extra key.
func TestCampaignSyncWithNamePostsBothKeys(t *testing.T) {
	srv, bodies := stubBrandCreateCapturing(t, "CEXMPL1")

	_, _, err := runBrandCmd(t, srv, "campaign", "sync", "CEXMPL1", "--campaign-name", "Acme Notifications")
	if err != nil {
		t.Fatalf("campaign sync: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if len(sent) != 2 || sent["campaignId"] != "CEXMPL1" || sent["campaignName"] != "Acme Notifications" {
		t.Errorf("posted body = %v, want exactly campaignId and campaignName", sent)
	}
}

// Test 13c: sync offers no --wait flag.
func TestCampaignSyncHasNoWaitFlag(t *testing.T) {
	if f := campaignSyncCmd.Flags().Lookup("wait"); f != nil {
		t.Errorf("campaignSyncCmd has a --wait flag, want none: %+v", f)
	}
}

// TestCampaignSyncRejectsWrongArgCount covers ExactArgs(1): zero and two
// positionals are both rejected.
func TestCampaignSyncRejectsWrongArgCount(t *testing.T) {
	for _, args := range [][]string{
		{"campaign", "sync"},
		{"campaign", "sync", "C1", "STRAY"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, _, err := runBrandCmd(t, nil, args...); err == nil {
				t.Fatal("want an argument error")
			}
		})
	}
}
