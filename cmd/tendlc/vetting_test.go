package tendlc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// stubVettingList answers GET .../vettings with one ACTIVE vetting on a
// single, non-truncated page.
func stubVettingList(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"bandwidthId":"V1","vettingStatus":"ACTIVE"}],` +
			`"page":{"pageNumber":0,"pageSize":50,"totalElements":1,"totalPages":1}}`))
	})
}

// stubVettingListTruncated answers with a page reporting more records exist
// than were returned, so warnIfTruncated fires.
func stubVettingListTruncated(t *testing.T) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"bandwidthId":"V1","vettingStatus":"ACTIVE"}],` +
			`"page":{"pageNumber":0,"pageSize":1,"totalElements":5,"totalPages":5}}`))
	})
}

// stubVettingRequestCapturing answers POST .../vettings with a 202 carrying
// idKey:idValue, and records each request's raw JSON body and path.
func stubVettingRequestCapturing(t *testing.T, idKey, idValue string) (*httptest.Server, *[]string, *[]string) {
	var bodies, paths []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, `{"data":{%q:%q}}`, idKey, idValue)
	})
	return srv, &bodies, &paths
}

// stubVettingImportCapturing answers PUT .../vettings/{id} with a 202
// carrying idKey:idValue, recording each request's body and path.
func stubVettingImportCapturing(t *testing.T, idKey, idValue string) (*httptest.Server, *[]string, *[]string) {
	var bodies, paths []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, `{"data":{%q:%q}}`, idKey, idValue)
	})
	return srv, &bodies, &paths
}

// stubVettingRequestThenPoll answers POST .../vettings with a 202 carrying
// idKey:idValue and GET .../vettings with a single list entry whose
// bandwidthId matches idValue and whose vettingStatus is status — enough for
// a --wait test that settles on the very first poll.
func stubVettingRequestThenPoll(t *testing.T, idKey, idValue, status string) *httptest.Server {
	return newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"data":{%q:%q}}`, idKey, idValue)
			return
		}
		_, _ = fmt.Fprintf(w, `{"data":[{"bandwidthId":%q,"vettingStatus":%q}],`+
			`"page":{"pageNumber":0,"pageSize":50,"totalElements":1,"totalPages":1}}`, idValue, status)
	})
}

// Test 1: missing --evp and --class aggregate into one exit-6 error naming
// both, and no request is made — runBrandCmd's `service` seam Fatals if
// invoked with a nil server.
func TestVettingRequestMissingRequiredFlagsAggregate(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "vetting", "request", "BGJR2BA", "--confirm")
	if err == nil {
		t.Fatal("want an error for missing --evp and --class")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "evp") || !strings.Contains(err.Error(), "class") {
		t.Errorf("error = %q, want it to name both missing flags", err.Error())
	}
}

// Test 2: --class RCS is accepted. This is the regression guard for the
// undocumented enum value — production accepts RCS despite its absence from
// the published enumVettingClass, and this must not be "corrected" away.
func TestVettingRequestAcceptsRCSClass(t *testing.T) {
	srv, bodies, _ := stubVettingRequestCapturing(t, "vettingBandwidthId", "V1")
	_, _, err := runBrandCmd(t, srv, "vetting", "request", "BGJR2BA",
		"--evp", "AEGIS", "--class", "RCS", "--confirm")
	if err != nil {
		t.Fatalf("vetting request --class RCS: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly one POST, got %d", len(*bodies))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if sent["vettingClass"] != "RCS" {
		t.Errorf("posted body = %v, want vettingClass RCS", sent)
	}
}

// Test 3: an invalid --class exits 6 listing the valid classes, with zero
// requests made.
func TestVettingRequestRejectsInvalidClass(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "vetting", "request", "BGJR2BA",
		"--evp", "AEGIS", "--class", "NOT_A_CLASS", "--confirm")
	if err == nil {
		t.Fatal("want an error for an invalid --class")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	for _, want := range []string{"STANDARD", "ENHANCED", "POLITICAL", "AUTHPLUS", "RCS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to list valid class %q", err.Error(), want)
		}
	}
}

// Test 4: an invalid --evp exits 6 listing the valid providers, with zero
// requests made.
func TestVettingRequestRejectsInvalidEvp(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "vetting", "request", "BGJR2BA",
		"--evp", "NOPE", "--class", "STANDARD", "--confirm")
	if err == nil {
		t.Fatal("want an error for an invalid --evp")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	for _, want := range []string{"AEGIS", "CV", "WMC"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to list valid provider %q", err.Error(), want)
		}
	}
}

// Test 4b: a missing --evp combined with an invalid --class combines into
// one error naming both. This is validateVettingRequest's one-missing/
// one-invalid aggregation branch — the same defect class Tasks 2 and 3 both
// shipped and both got a regression guard for; this was the one instance
// left unguarded at the final whole-branch review (item C3).
func TestVettingRequestAggregatesOneMissingOneInvalidFlag(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "vetting", "request", "BGJR2BA",
		"--class", "NOT_A_CLASS", "--confirm")
	if err == nil {
		t.Fatal("want an error for a missing --evp combined with an invalid --class")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "STANDARD") {
		t.Errorf("error = %q, want it to list the valid classes (the invalid --class violation)", err.Error())
	}
	if !strings.Contains(err.Error(), "--evp") {
		t.Errorf("error = %q, want it to also name the missing --evp flag", err.Error())
	}
}

// Test 5: vetting request without --confirm exits 6, makes ZERO HTTP
// requests, and the message mentions the order is billable.
func TestVettingRequestWithoutConfirmMakesNoRequests(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "vetting", "request", "BGJR2BA", "--evp", "AEGIS", "--class", "STANDARD")
	if err == nil {
		t.Fatal("want an error when --confirm is missing")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "billable") {
		t.Errorf("error = %q, want it to mention the order is billable", err.Error())
	}
}

// Test 6: with --confirm, request POSTs {"evpId":..., "vettingClass":...} and
// prints the 202 receipt on stdout.
func TestVettingRequestWithConfirmPostsBodyAndPrintsReceipt(t *testing.T) {
	srv, bodies, paths := stubVettingRequestCapturing(t, "vettingBandwidthId", "V1")
	out, _, err := runBrandCmd(t, srv, "vetting", "request", "BGJR2BA",
		"--evp", "AEGIS", "--class", "STANDARD", "--confirm")
	if err != nil {
		t.Fatalf("vetting request --confirm: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly one POST, got %d", len(*bodies))
	}
	if !strings.HasSuffix((*paths)[0], "/brands/BGJR2BA/vettings") {
		t.Errorf("path = %q, want POST to .../brands/BGJR2BA/vettings", (*paths)[0])
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if sent["evpId"] != "AEGIS" || sent["vettingClass"] != "STANDARD" {
		t.Errorf("posted body = %v, want evpId AEGIS and vettingClass STANDARD", sent)
	}
	got := decodeStdout(t, out)
	if got["vettingBandwidthId"] != "V1" {
		t.Errorf("stdout = %v, want vettingBandwidthId V1", got)
	}
	if got["status"] != "accepted" {
		t.Errorf("stdout = %v, want status accepted", got)
	}
}

// Test 7: import PUTs to /vettings/{vettingId} with {"evpId": ...}, and
// includes vettingToken only when the flag was actually passed.
func TestVettingImportPutsEvpAndOnlyIncludesTokenWhenPassed(t *testing.T) {
	t.Run("without token", func(t *testing.T) {
		srv, bodies, paths := stubVettingImportCapturing(t, "bandwidthId", "V1")
		_, _, err := runBrandCmd(t, srv, "vetting", "import", "BGJR2BA", "V1", "--evp", "CV")
		if err != nil {
			t.Fatalf("vetting import: %v", err)
		}
		if len(*bodies) != 1 {
			t.Fatalf("want exactly one PUT, got %d", len(*bodies))
		}
		if !strings.HasSuffix((*paths)[0], "/brands/BGJR2BA/vettings/V1") {
			t.Errorf("path = %q, want PUT to .../brands/BGJR2BA/vettings/V1", (*paths)[0])
		}
		var sent map[string]any
		if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		if sent["evpId"] != "CV" {
			t.Errorf("posted body = %v, want evpId CV", sent)
		}
		if _, present := sent["vettingToken"]; present {
			t.Errorf("posted body = %v, want no vettingToken key when --vetting-token was not passed", sent)
		}
	})

	t.Run("with token", func(t *testing.T) {
		srv, bodies, _ := stubVettingImportCapturing(t, "bandwidthId", "V1")
		_, _, err := runBrandCmd(t, srv, "vetting", "import", "BGJR2BA", "V1",
			"--evp", "CV", "--vetting-token", "TOK123")
		if err != nil {
			t.Fatalf("vetting import --vetting-token: %v", err)
		}
		var sent map[string]any
		if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		if sent["vettingToken"] != "TOK123" {
			t.Errorf("posted body = %v, want vettingToken TOK123", sent)
		}
	})
}

// vetting import has no --confirm: recording an already-performed vetting is
// not billable, unlike 'vetting request'. This locks in that the command
// does not define the flag at all, mirroring
// TestBrandResend2FANeedsNoConfirm.
func TestVettingImportHasNoConfirmFlag(t *testing.T) {
	if f := vettingImportCmd.Flags().Lookup("confirm"); f != nil {
		t.Errorf("vettingImportCmd has a --confirm flag, want none: %+v", f)
	}
}

// Test 8: vetting list on a brand with one ACTIVE vetting prints it, and the
// truncation warning goes to stderr only.
func TestVettingListPrintsEntryAndWarnsOnTruncationViaStderrOnly(t *testing.T) {
	out, _, err := runBrandCmd(t, stubVettingList(t), "vetting", "list", "BGJR2BA")
	if err != nil {
		t.Fatalf("vetting list: %v", err)
	}
	if !strings.Contains(out, "ACTIVE") || !strings.Contains(out, "V1") {
		t.Errorf("stdout = %q, want the ACTIVE vetting entry", out)
	}

	out, errOut, err := runBrandCmd(t, stubVettingListTruncated(t), "vetting", "list", "BGJR2BA", "--limit", "1")
	if err != nil {
		t.Fatalf("vetting list (truncated): %v", err)
	}
	if strings.Contains(out, "pass --all") {
		t.Error("truncation warning leaked into stdout; stdout must stay parseable")
	}
	if !strings.Contains(errOut, "pass --all") {
		t.Errorf("stderr should carry the truncation warning, got %q", errOut)
	}
}

// Test 9: every vetting command rejects the wrong positional count, before
// any request is made.
func TestVettingCommandsRejectWrongPositionalCount(t *testing.T) {
	cases := [][]string{
		{"vetting", "list"},
		{"vetting", "list", "B1", "STRAY"},
		{"vetting", "request"},
		{"vetting", "request", "B1", "STRAY"},
		{"vetting", "import"},
		{"vetting", "import", "B1"},
		{"vetting", "import", "B1", "V1", "STRAY"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, _, err := runBrandCmd(t, nil, args...); err == nil {
				t.Fatal("want an argument error")
			}
		})
	}
}

// Test 10: the 202 receipt preserves the API's own ID field name. A stub
// returning vettingBandwidthId (the spec's documented key for the accept)
// must produce a receipt carrying vettingBandwidthId — NOT renamed to
// bandwidthId (the key the vettings list uses for the same concept).
// Normalizing would be tidier and would be wrong: it would misreport which
// field the API actually sent.
func TestVettingRequestReceiptPreservesAPIFieldName(t *testing.T) {
	srv, _, _ := stubVettingRequestCapturing(t, "vettingBandwidthId", "V-XYZ")
	out, _, err := runBrandCmd(t, srv, "vetting", "request", "BGJR2BA",
		"--evp", "AEGIS", "--class", "STANDARD", "--confirm")
	if err != nil {
		t.Fatalf("vetting request: %v", err)
	}
	got := decodeStdout(t, out)
	if got["vettingBandwidthId"] != "V-XYZ" {
		t.Errorf("stdout = %v, want vettingBandwidthId V-XYZ preserved under its own name", got)
	}
	if _, present := got["bandwidthId"]; present {
		t.Errorf("stdout = %v, want no renamed bandwidthId key", got)
	}
}

// TestVettingRequestWaitPollsToActive exercises the --wait wiring end to end:
// the 202 returns vettingBandwidthId, but the vettings list (the only read
// surface for a vetting's status) reports the same ID under bandwidthId —
// the field-name quirk this command set preserves rather than normalizes.
// --wait must still match the two up by VALUE across the differently-named
// keys and print the final ACTIVE resource.
func TestVettingRequestWaitPollsToActive(t *testing.T) {
	srv := stubVettingRequestThenPoll(t, "vettingBandwidthId", "V1", "ACTIVE")
	out, _, err := runBrandCmd(t, srv, "vetting", "request", "BGJR2BA",
		"--evp", "AEGIS", "--class", "STANDARD", "--confirm", "--wait", "--timeout", "5")
	if err != nil {
		t.Fatalf("vetting request --wait: %v", err)
	}
	got := decodeStdout(t, out)
	if got["vettingStatus"] != "ACTIVE" {
		t.Errorf("stdout = %v, want the final ACTIVE resource", got)
	}
	if got["bandwidthId"] != "V1" {
		t.Errorf("stdout = %v, want bandwidthId V1 from the list entry", got)
	}
}

// TestVettingRoleGate403MapsToExitFour exercises roleGateError on all three
// vetting commands.
func TestVettingRoleGate403MapsToExitFour(t *testing.T) {
	body := `{"errors":[{"description":"does not have access rights"}]}`
	cases := []struct {
		name string
		args []string
	}{
		{"list", []string{"vetting", "list", "BGJR2BA"}},
		{"request", []string{"vetting", "request", "BGJR2BA", "--evp", "AEGIS", "--class", "STANDARD", "--confirm"}},
		{"import", []string{"vetting", "import", "BGJR2BA", "V1", "--evp", "AEGIS"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runBrandCmd(t, stubBrandErr(t, 403, body), tt.args...)
			if err == nil {
				t.Fatal("want an error on 403")
			}
			if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
				t.Errorf("exit code = %d, want %d", code, cmdutil.ExitConflict)
			}
		})
	}
}

// THE load-bearing regression test for buildVettingReceipt's no-vetting-ID
// fallback — the equivalent, in this file, of brand_create_test.go's
// TestBrandCreateNoBandwidthIDPrintsBodyWithKeyPreserved. A response carrying
// neither bandwidthId nor vettingBandwidthId must still print the real body
// as an OBJECT, preserving its single key — not unwrap it to a bare array.
// Before the fallback was switched from output.StdoutAuto to output.Stdout,
// --plain would run this through FlattenResponse, which unwraps ANY
// single-key map, silently dropping the "orders" key and printing a bare
// array instead of the object it came from.
func TestVettingRequestNoVettingIDPrintsBodyWithKeyPreserved(t *testing.T) {
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"data":{"orders":[{"orderId":"O1"}]}}`))
	})

	out, _, err := runBrandCmd(t, srv, "vetting", "request", "BGJR2BA",
		"--evp", "AEGIS", "--class", "STANDARD", "--confirm", "--plain")
	if err == nil {
		t.Fatal("want an error when the response carries no vetting ID")
	}
	if !strings.Contains(err.Error(), "vetting ID") {
		t.Errorf("error = %q, want it to name the missing vetting ID", err.Error())
	}
	got := decodeStdout(t, out)
	orders, ok := got["orders"]
	if !ok {
		t.Fatalf("stdout = %q, want the \"orders\" key preserved, not unwrapped to a bare array", out)
	}
	if arr, ok := orders.([]any); !ok || len(arr) != 1 {
		t.Errorf("stdout orders = %v, want a one-element array", orders)
	}
}
