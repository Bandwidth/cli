package tendlc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	cpsvc "github.com/Bandwidth/cli/internal/customerprofile"
)

// validPrivateProfitArgs returns a full, valid PRIVATE_PROFIT flag set for
// `brand create`, so each test only needs to state what it adds or omits.
func validPrivateProfitArgs(extra ...string) []string {
	args := []string{
		"brand", "create",
		"--customer-profile-id", "CP123",
		"--brand-type", "PRIVATE_PROFIT",
		"--display-name", "Acme Corp",
		"--company-name", "Acme Corporation",
		"--street", "123 Main St",
		"--city", "Raleigh",
		"--state", "NC",
		"--postal-code", "27601",
		"--country-code-a3", "USA",
		"--phone", "+18885551234",
		"--email", "ops@acme.com",
		"--vertical", "RETAIL",
		"--ein", "123456789",
		"--ein-issuing-country-code-a3", "USA",
	}
	return append(args, extra...)
}

// stubProfileServer answers every request with the given status and body,
// for exercising the create pre-flight's customer-profile read.
func stubProfileServer(t *testing.T, code int, body string) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	})
}

// stubProfileService swaps the customerProfileService seam to point at srv,
// restoring the original after the test. Kept independent of runBrandCmd's
// `service` swap because the pre-flight check is its own seam — see
// customerProfileService's doc comment in brand_create.go — so a test can
// stub the profile read and the brand-create request separately.
func stubProfileService(t *testing.T, srv *httptest.Server) {
	t.Helper()
	orig := customerProfileService
	customerProfileService = func(cmd *cobra.Command) (*cpsvc.Service, error) {
		if srv == nil {
			t.Fatal("pre-flight made a request but no profile stub was provided")
		}
		return cpsvc.NewService(api.NewClientNoAuth(srv.URL), "9901287"), nil
	}
	t.Cleanup(func() { customerProfileService = orig })
}

// stubBrandCreateCapturing answers POST /brands with a 202 carrying
// bandwidthID, recording each request's raw JSON body so a test can assert
// exactly what was sent.
func stubBrandCreateCapturing(t *testing.T, bandwidthID string) (*httptest.Server, *[]string) {
	var bodies []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, `{"data":{"bandwidthId":%q}}`, bandwidthID)
	})
	return srv, &bodies
}

// stubBrandCreateThenPoll answers POST /brands with a 202 accepting
// bandwidthID, and GET /brands/{id} with identityStatus on every poll —
// enough for a --wait test that settles on the very first poll, so it never
// has to sleep out the poll interval.
func stubBrandCreateThenPoll(t *testing.T, bandwidthID, identityStatus string) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"data":{"bandwidthId":%q}}`, bandwidthID)
			return
		}
		_, _ = fmt.Fprintf(w, `{"data":{"bandwidthId":%q,"brandIdentityStatus":%q}}`, bandwidthID, identityStatus)
	})
}

func decodeStdout(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &m); err != nil {
		t.Fatalf("stdout is not JSON (%v): %q", err, s)
	}
	return m
}

// Test 1: every missing required flag is named in one exit-6 error, and
// nothing is ever sent — the fake `service` and `customerProfileService`
// closures below both call t.Fatal if invoked, so this test cannot pass by
// accident even if RunE regressed to calling one of them before validating.
func TestBrandCreateMissingRequiredFlagsAggregate(t *testing.T) {
	stubProfileService(t, nil)
	_, _, err := runBrandCmd(t, nil, "brand", "create")
	if err == nil {
		t.Fatal("want an error for missing required flags")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	for _, want := range []string{"customer-profile-id", "display-name", "street", "city",
		"state", "postal-code", "country-code-a3", "phone", "email", "brand-type"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, missing flag %q", err.Error(), want)
		}
	}
}

// Test 2: PUBLIC_PROFIT additionally requires the four schema-optional
// fields. Supplying everything else but those four must still fail, exit 6,
// with zero requests made.
func TestBrandCreatePublicProfitRequiresFourExtraFlags(t *testing.T) {
	stubProfileService(t, nil)
	args := []string{
		"brand", "create",
		"--customer-profile-id", "CP123",
		"--brand-type", "PUBLIC_PROFIT",
		"--display-name", "Acme Corp",
		"--company-name", "Acme Corporation",
		"--street", "123 Main St",
		"--city", "Raleigh",
		"--state", "NC",
		"--postal-code", "27601",
		"--country-code-a3", "USA",
		"--phone", "+18885551234",
		"--email", "ops@acme.com",
		"--vertical", "RETAIL",
		"--ein", "123456789",
		"--ein-issuing-country-code-a3", "USA",
	}
	_, _, err := runBrandCmd(t, nil, args...)
	if err == nil {
		t.Fatal("want an error for missing PUBLIC_PROFIT-only flags")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	for _, want := range []string{"stock-symbol", "stock-exchange", "website", "business-contact-email"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, missing flag %q", err.Error(), want)
		}
	}
}

// Test 3: a valid create posts the built body and prints the 202 receipt
// carrying bandwidthId.
func TestBrandCreatePostsBuiltBodyAndPrintsReceipt(t *testing.T) {
	stubProfileService(t, stubProfileServer(t, http.StatusOK, `{"data":{"id":"CP123"}}`))
	srv, bodies := stubBrandCreateCapturing(t, "WNEW1")

	out, _, err := runBrandCmd(t, srv, validPrivateProfitArgs()...)
	if err != nil {
		t.Fatalf("brand create: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly one POST, got %d", len(*bodies))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if sent["customerProfileId"] != "CP123" || sent["displayName"] != "Acme Corp" || sent["brandType"] != "PRIVATE_PROFIT" {
		t.Errorf("posted body = %v, missing built fields", sent)
	}

	got := decodeStdout(t, out)
	if got["bandwidthId"] != "WNEW1" {
		t.Errorf("stdout = %v, want bandwidthId WNEW1", got)
	}
	if got["status"] != "accepted" {
		t.Errorf("stdout = %v, want status accepted", got)
	}
}

// Test 4: --wait polls to VERIFIED and prints the final brand resource, not
// the synthetic receipt.
func TestBrandCreateWaitPollsToVerified(t *testing.T) {
	stubProfileService(t, stubProfileServer(t, http.StatusOK, `{"data":{"id":"CP123"}}`))
	srv := stubBrandCreateThenPoll(t, "WNEW1", "VERIFIED")

	out, _, err := runBrandCmd(t, srv, validPrivateProfitArgs("--wait", "--timeout", "5")...)
	if err != nil {
		t.Fatalf("brand create --wait: %v", err)
	}
	got := decodeStdout(t, out)
	if got["brandIdentityStatus"] != "VERIFIED" {
		t.Errorf("stdout = %v, want the final VERIFIED resource", got)
	}
	if got["bandwidthId"] != "WNEW1" {
		t.Errorf("stdout = %v, want bandwidthId WNEW1", got)
	}
}

// Test 5: --wait on a brand that settles UNVERIFIED exits 4, still prints
// the resource, and puts remediation on stderr.
func TestBrandCreateWaitUnverifiedExitsConflictWithRemediation(t *testing.T) {
	stubProfileService(t, stubProfileServer(t, http.StatusOK, `{"data":{"id":"CP123"}}`))
	srv := stubBrandCreateThenPoll(t, "WNEW1", "UNVERIFIED")

	out, errOut, err := runBrandCmd(t, srv, validPrivateProfitArgs("--wait", "--timeout", "5")...)
	if err == nil {
		t.Fatal("want an error for a brand that settles UNVERIFIED")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitConflict)
	}
	got := decodeStdout(t, out)
	if got["brandIdentityStatus"] != "UNVERIFIED" {
		t.Errorf("stdout = %v, want the UNVERIFIED resource still printed", got)
	}
	if !strings.Contains(errOut, "reverify") {
		t.Errorf("stderr should carry UNVERIFIED remediation, got %q", errOut)
	}
}

// Test 6: a --wait timeout exits 5 and still emits a receipt carrying
// bandwidthId and a resume command — the whole point of awaitTerminal.
// --timeout 0 makes the deadline already past by the time the first poll
// returns, so this fails fast instead of waiting out the real poll interval.
func TestBrandCreateWaitTimeoutStillEmitsReceipt(t *testing.T) {
	stubProfileService(t, stubProfileServer(t, http.StatusOK, `{"data":{"id":"CP123"}}`))
	srv := stubBrandCreateThenPoll(t, "WNEW1", "REGISTERING")

	out, _, err := runBrandCmd(t, srv, validPrivateProfitArgs("--wait", "--timeout", "0")...)
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitTimeout {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitTimeout)
	}
	got := decodeStdout(t, out)
	if got["bandwidthId"] != "WNEW1" {
		t.Fatalf("timeout must still print the ID, got %v", got)
	}
	if got["resume"] != "band tendlc brand get WNEW1" {
		t.Errorf("timeout receipt must carry a resume command, got %v", got)
	}
}

// Test 7: a pre-flight 404 on the customer profile stops the create at exit
// 3 and makes NO request to /brands. The create stub below fails the
// assertion (not the test directly, since t.Fatal from the server's
// goroutine is unsafe) by recording any request it receives.
func TestBrandCreatePreflight404BlocksCreate(t *testing.T) {
	stubProfileService(t, stubProfileServer(t, http.StatusNotFound, `{"errors":[{"description":"customer profile not found"}]}`))
	srv, bodies := stubBrandCreateCapturing(t, "WSHOULD-NOT-EXIST")

	_, _, err := runBrandCmd(t, srv, validPrivateProfitArgs()...)
	if err == nil {
		t.Fatal("want an error when the customer profile pre-flight 404s")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitNotFound {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitNotFound)
	}
	if !strings.Contains(err.Error(), "CP123") {
		t.Errorf("error = %q, want it to name the profile ID", err.Error())
	}
	if len(*bodies) != 0 {
		t.Errorf("want zero requests to /brands, got %d", len(*bodies))
	}
}

// Test 8: a pre-flight 403 proceeds with the create anyway and warns on
// stderr — Customer Profiles Access is a role separate from Campaign
// Management, so a caller entitled to create brands must not be blocked by a
// check they lack permission to run.
func TestBrandCreatePreflight403ProceedsWithWarning(t *testing.T) {
	stubProfileService(t, stubProfileServer(t, http.StatusForbidden, `{"errors":[{"description":"does not have access rights"}]}`))
	srv, bodies := stubBrandCreateCapturing(t, "WNEW1")

	out, errOut, err := runBrandCmd(t, srv, validPrivateProfitArgs()...)
	if err != nil {
		t.Fatalf("brand create: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want the create to proceed despite the 403 pre-flight, got %d requests", len(*bodies))
	}
	got := decodeStdout(t, out)
	if got["bandwidthId"] != "WNEW1" {
		t.Errorf("stdout = %v, want the create receipt", got)
	}
	if !strings.Contains(errOut, "CP123") {
		t.Errorf("stderr = %q, want a warning naming the profile ID", errOut)
	}
}

// Test 9: a stray positional is rejected by cobra.NoArgs before RunE ever
// runs, so no request is made to either seam.
func TestBrandCreateRejectsStrayPositional(t *testing.T) {
	stubProfileService(t, nil)
	srv, bodies := stubBrandCreateCapturing(t, "WNEW1")

	_, _, err := runBrandCmd(t, srv, append([]string{"brand", "create", "STRAY"},
		validPrivateProfitArgs()[2:]...)...)
	if err == nil {
		t.Fatal("want an argument error for a stray positional")
	}
	if len(*bodies) != 0 {
		t.Errorf("want zero requests, got %d", len(*bodies))
	}
}

// Test 10: refresh posts exactly {"brandId": "BGJR2BA"} — no other keys. A
// refresh body carrying any extra key turns it back into a create.
func TestBrandRefreshPostsExactBody(t *testing.T) {
	srv, bodies := stubBrandCreateCapturing(t, "WET8JUY8H0")

	out, _, err := runBrandCmd(t, srv, "brand", "refresh", "BGJR2BA")
	if err != nil {
		t.Fatalf("brand refresh: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly one POST, got %d", len(*bodies))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if len(sent) != 1 || sent["brandId"] != "BGJR2BA" {
		t.Errorf("posted body = %v, want exactly {\"brandId\":\"BGJR2BA\"}", sent)
	}
	got := decodeStdout(t, out)
	if got["bandwidthId"] != "WET8JUY8H0" {
		t.Errorf("stdout = %v, want the refresh receipt", got)
	}
}

// Test 11: refresh offers no --wait flag. It writes against a brand usually
// already in a terminal state, so "poll until settled" would return
// immediately and report success before the change applied.
func TestBrandRefreshHasNoWaitFlag(t *testing.T) {
	if f := brandRefreshCmd.Flags().Lookup("wait"); f != nil {
		t.Errorf("brandRefreshCmd has a --wait flag, want none: %+v", f)
	}
}
