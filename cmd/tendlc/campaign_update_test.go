package tendlc

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// liveCampaignForUpdate mirrors the shape GET /campaigns/{id} actually
// returns, for a direct (imported: false) or imported campaign depending on
// the argument. It deliberately carries "someFutureField", a key the CLI
// does not model at all, so the description-only losslessness test below has
// something genuinely unmodeled to check for.
func liveCampaignForUpdate(imported bool) map[string]any {
	return map[string]any{
		"bandwidthId":     "C9900000BW",
		"campaignId":      "CEXMPL1",
		"brandId":         "BEXMPL1",
		"imported":        imported,
		"campaignName":    "Acme Notifications",
		"description":     "Sends account notifications to opted-in subscribers.",
		"messageFlow":     "Customer opts in via web form; campaign sends account notifications only.",
		"sample1":         "Your account balance is now available. Reply STOP to opt out.",
		"ageGated":        false,
		"status":          "REGISTERED",
		"someFutureField": "keep me",
	}
}

// stubCampaignUpdateServer answers GET /campaigns/{id} with getBody wrapped
// in the standard envelope, and PUT with either putStatus's error body (if
// putStatus != 200) or the measured production acceptance shape -- a bare
// {bandwidthId, campaignId}, carrying getBody's own IDs, NOT an echo of the
// sent body. It still records every PUT body's raw JSON so a test can assert
// on the request the CLI actually sent, independent of what the response
// looks like. Modeled on stubBrandUpdateServer in brand_update_test.go.
func stubCampaignUpdateServer(t *testing.T, getBody map[string]any, putStatus int) (*httptest.Server, *[]string) {
	t.Helper()
	getJSON, err := json.Marshal(map[string]any{"data": getBody})
	if err != nil {
		t.Fatalf("marshaling GET fixture: %v", err)
	}
	acceptJSON, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"bandwidthId": getBody["bandwidthId"],
			"campaignId":  getBody["campaignId"],
		},
	})
	if err != nil {
		t.Fatalf("marshaling PUT acceptance fixture: %v", err)
	}
	var putBodies []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write(getJSON)
			return
		}
		b, _ := io.ReadAll(r.Body)
		putBodies = append(putBodies, string(b))
		if putStatus != http.StatusOK {
			w.WriteHeader(putStatus)
			_, _ = w.Write([]byte(`{"errors":[{"description":"conflict"}]}`))
			return
		}
		_, _ = w.Write(acceptJSON)
	})
	return srv, &putBodies
}

// Test 1: no field flags at all is exit 6 and makes no request whatsoever --
// runBrandCmd's `service` seam Fatals if invoked with a nil server, so this
// cannot pass by accident even if the "nothing to update" check moved after
// the service/GET call.
func TestCampaignUpdateNoFieldFlagsExitsSixWithZeroRequests(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "campaign", "update", "CEXMPL1")
	if err == nil {
		t.Fatal("want an error when no field flags are passed")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "nothing to update") {
		t.Errorf("error = %q, want it to say nothing to update", err.Error())
	}
}

// Test 2: THE load-bearing regression test for this task. A description-only
// update on a DIRECT campaign must preserve every other field on the PUT
// body -- including messageFlow, sample1, and someFutureField, a key the CLI
// does not model at all. This is the guard against silent data loss on a
// full-replacement PUT.
func TestCampaignUpdateDescriptionOnlyPreservesEveryOtherField(t *testing.T) {
	srv, bodies := stubCampaignUpdateServer(t, liveCampaignForUpdate(false), http.StatusOK)

	_, _, err := runBrandCmd(t, srv, "campaign", "update", "CEXMPL1", "--description", "Updated description.", "--plain")
	if err != nil {
		t.Fatalf("campaign update: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly one PUT, got %d", len(*bodies))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("PUT body is not JSON: %v", err)
	}
	if sent["description"] != "Updated description." {
		t.Errorf("sent description = %v, want Updated description.", sent["description"])
	}
	if sent["messageFlow"] != "Customer opts in via web form; campaign sends account notifications only." {
		t.Errorf("sent messageFlow = %v, want it preserved", sent["messageFlow"])
	}
	if sent["sample1"] != "Your account balance is now available. Reply STOP to opt out." {
		t.Errorf("sent sample1 = %v, want it preserved", sent["sample1"])
	}
	if sent["campaignName"] != "Acme Notifications" {
		t.Errorf("sent campaignName = %v, want it preserved", sent["campaignName"])
	}
	if sent["someFutureField"] != "keep me" {
		t.Errorf("sent someFutureField = %v, want the unmodeled field preserved", sent["someFutureField"])
	}
}

// Test 3: a changed boolean reaches the wire as a real bool -- an explicitly
// passed --age-gated=false must be PRESENT and false, not merely falsy or
// absent. Checked via comma-ok so this would fail if the field were dropped
// entirely (Go's zero value for a missing map key is also false).
func TestCampaignUpdateAgeGatedFalseReachesBodyPresentAndFalse(t *testing.T) {
	fixture := liveCampaignForUpdate(false)
	fixture["ageGated"] = true
	srv, bodies := stubCampaignUpdateServer(t, fixture, http.StatusOK)

	_, _, err := runBrandCmd(t, srv, "campaign", "update", "CEXMPL1", "--age-gated=false", "--plain")
	if err != nil {
		t.Fatalf("campaign update: %v", err)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("PUT body is not JSON: %v", err)
	}
	v, present := sent["ageGated"]
	if !present {
		t.Fatalf("sent body = %v, want ageGated present", sent)
	}
	if v != false {
		t.Errorf("sent ageGated = %v, want false", v)
	}
}

// Test 4: THE other load-bearing regression test for this task. An imported
// campaign accepts only --campaign-name: any other explicitly-changed flag
// is rejected with exit 6 naming it, with NO PUT issued -- and passing only
// --campaign-name issues a PUT whose body has exactly one key.
func TestCampaignUpdateImportedAcceptsOnlyCampaignName(t *testing.T) {
	t.Run("other flags rejected, no PUT", func(t *testing.T) {
		srv, bodies := stubCampaignUpdateServer(t, liveCampaignForUpdate(true), http.StatusOK)

		_, _, err := runBrandCmd(t, srv, "campaign", "update", "CEXMPL1",
			"--description", "New description.", "--age-gated=true", "--plain")
		if err == nil {
			t.Fatal("want an error when a non-campaign-name flag changes on an imported campaign")
		}
		if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
			t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
		}
		if !strings.Contains(err.Error(), "description") || !strings.Contains(err.Error(), "age-gated") {
			t.Errorf("error = %q, want it to name both rejected flags", err.Error())
		}
		if len(*bodies) != 0 {
			t.Errorf("want zero PUT requests, got %d", len(*bodies))
		}
	})

	t.Run("campaign-name only sends exactly one key", func(t *testing.T) {
		srv, bodies := stubCampaignUpdateServer(t, liveCampaignForUpdate(true), http.StatusOK)

		_, _, err := runBrandCmd(t, srv, "campaign", "update", "CEXMPL1",
			"--campaign-name", "Acme Notifications v2", "--plain")
		if err != nil {
			t.Fatalf("campaign update: %v", err)
		}
		if len(*bodies) != 1 {
			t.Fatalf("want exactly one PUT, got %d", len(*bodies))
		}
		var sent map[string]any
		if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
			t.Fatalf("PUT body is not JSON: %v", err)
		}
		if len(sent) != 1 || sent["campaignName"] != "Acme Notifications v2" {
			t.Errorf("sent body = %v, want exactly {\"campaignName\":\"Acme Notifications v2\"}", sent)
		}
	})
}

// Test 5: a campaign whose GET response lacks "imported" entirely fails
// cleanly rather than guessing which arm of BuildCampaignUpdateRequest
// applies.
func TestCampaignUpdateMissingImportedFieldFailsCleanly(t *testing.T) {
	fixture := liveCampaignForUpdate(false)
	delete(fixture, "imported")
	srv, bodies := stubCampaignUpdateServer(t, fixture, http.StatusOK)

	_, _, err := runBrandCmd(t, srv, "campaign", "update", "CEXMPL1", "--description", "New description.", "--plain")
	if err == nil {
		t.Fatal("want an error when the campaign response has no usable imported field")
	}
	if len(*bodies) != 0 {
		t.Errorf("want zero PUT requests, got %d", len(*bodies))
	}
}

// Test 6: clearing --description on a direct campaign exits 6 before any
// PUT -- description is required on every campaign and cannot be cleared
// this way.
func TestCampaignUpdateClearingDescriptionExitsSixBeforePut(t *testing.T) {
	srv, bodies := stubCampaignUpdateServer(t, liveCampaignForUpdate(false), http.StatusOK)

	_, _, err := runBrandCmd(t, srv, "campaign", "update", "CEXMPL1", "--description", "", "--plain")
	if err == nil {
		t.Fatal("want an error when clearing a required field")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if len(*bodies) != 0 {
		t.Errorf("want zero PUT requests, got %d", len(*bodies))
	}
}

// Test 7: a 409 on the PUT means the campaign exists in Bandwidth but not
// yet in TCR -- exit 4, and the message points at 'campaign sync'.
func TestCampaignUpdateConflictOnPutMapsToTCRHint(t *testing.T) {
	srv, _ := stubCampaignUpdateServer(t, liveCampaignForUpdate(false), http.StatusConflict)

	_, _, err := runBrandCmd(t, srv, "campaign", "update", "CEXMPL1", "--description", "New description.", "--plain")
	if err == nil {
		t.Fatal("want an error on a 409 PUT")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitConflict)
	}
	if !strings.Contains(err.Error(), "TCR") {
		t.Errorf("error = %q, want it to mention TCR", err.Error())
	}
	if !strings.Contains(err.Error(), "campaign sync") {
		t.Errorf("error = %q, want it to suggest campaign sync", err.Error())
	}
}

// Test 8: update has no --wait and no --confirm flag -- campaign updates
// carry no fee and no identity reset, and the write lands against a
// campaign usually already in a terminal state.
func TestCampaignUpdateHasNoWaitAndNoConfirmFlag(t *testing.T) {
	if f := campaignUpdateCmd.Flags().Lookup("wait"); f != nil {
		t.Errorf("campaignUpdateCmd has a --wait flag, want none: %+v", f)
	}
	if f := campaignUpdateCmd.Flags().Lookup("confirm"); f != nil {
		t.Errorf("campaignUpdateCmd has a --confirm flag, want none: %+v", f)
	}
}

// Test 9: THE regression test for the acceptance-vs-resource bug brand
// update had before it was fixed. PUT /campaigns/{id} returns only a bare
// {bandwidthId, campaignId} acceptance, and the change itself may take
// several minutes to apply. A successful update must print an acceptance
// receipt carrying both IDs, status "accepted", and a note -- not the raw
// PUT response as though it were the updated campaign. This stub's PUT
// response deliberately does NOT echo the sent body (see
// stubCampaignUpdateServer), so if the command reverted to printing
// updated.Object(), this test would fail on the missing "status"/"note"
// fields; and even against an echoing stub, the explicit check that
// "description" is absent from the receipt would still catch a regression
// back to printing the raw response body.
func TestCampaignUpdatePrintsAcceptanceReceiptNotCampaignObject(t *testing.T) {
	srv, _ := stubCampaignUpdateServer(t, liveCampaignForUpdate(false), http.StatusOK)

	out, _, err := runBrandCmd(t, srv, "campaign", "update", "CEXMPL1", "--description", "New description.", "--plain")
	if err != nil {
		t.Fatalf("campaign update: %v", err)
	}
	got := decodeStdout(t, out)
	if got["bandwidthId"] != "C9900000BW" {
		t.Errorf("stdout = %v, want bandwidthId C9900000BW", got)
	}
	if got["campaignId"] != "CEXMPL1" {
		t.Errorf("stdout = %v, want campaignId CEXMPL1", got)
	}
	if got["status"] != "accepted" {
		t.Errorf("stdout = %v, want status accepted", got)
	}
	note, _ := got["note"].(string)
	if !strings.Contains(note, "campaign get") {
		t.Errorf("stdout note = %q, want it to point at campaign get", note)
	}
	if _, ok := got["description"]; ok {
		t.Errorf("stdout = %v, must not print the raw PUT body as though it were the updated campaign", got)
	}
}
