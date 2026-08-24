package tendlc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// stubBrandDeleteServer answers DELETE /brands/{id} with deleteStatus (and an
// empty body, matching a 204), and any subsequent GET (used by --wait's
// follow-up poll) with 404 — the delete having actually taken effect. It
// records every request method it sees.
func stubBrandDeleteServer(t *testing.T, deleteStatus int) (*httptest.Server, *[]string) {
	t.Helper()
	var methods []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodDelete {
			w.WriteHeader(deleteStatus)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"description":"brand not found"}]}`))
	})
	return srv, &methods
}

// stubBrandDeleteStillExistsServer answers DELETE with deleteStatus, but
// every subsequent GET with 200 and a body proving the brand is still there
// — matching production's real ~40s propagation delay, where the brand
// outlives the DELETE response for a while. Used to exercise the --wait
// timeout path, where the follow-up read never 404s before the deadline.
func stubBrandDeleteStillExistsServer(t *testing.T, deleteStatus int) (*httptest.Server, *[]string) {
	t.Helper()
	var methods []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodDelete {
			w.WriteHeader(deleteStatus)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"brandId":"BEXMPL6","bandwidthId":"WET8JUY8H0"}}`))
	})
	return srv, &methods
}

// Test 10: delete without --confirm exits 6 and makes ZERO HTTP requests —
// runBrandCmd's `service` seam Fatals if it is ever invoked with a nil
// server, so this fails loudly (not silently) if the confirm gate ever moves
// after the service/DELETE call.
func TestBrandDeleteWithoutConfirmMakesNoRequests(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "brand", "delete", "BEXMPL6")
	if err == nil {
		t.Fatal("want an error when --confirm is missing")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "cannot be undone") {
		t.Errorf("error = %q, want it to say the delete cannot be undone", err.Error())
	}
	// The endpoint docs claim the delete cascades to the customer profile.
	// Measured against production: it does not — both test brands' backing
	// profiles remained retrievable (softDeleted:false) after the brand was
	// deleted. The refusal message must not repeat that false claim, and
	// must instead tell the caller to remove the profile separately.
	if strings.Contains(err.Error(), "also deletes") || strings.Contains(err.Error(), "AND its associated") {
		t.Errorf("error = %q, must not claim the delete cascades to the customer profile", err.Error())
	}
	if !strings.Contains(err.Error(), "does NOT delete the associated customer profile") {
		t.Errorf("error = %q, want it to state the profile is NOT deleted", err.Error())
	}
	if !strings.Contains(err.Error(), "customer-profile delete") {
		t.Errorf("error = %q, want it to point at 'band customer-profile delete' to remove the profile separately", err.Error())
	}
}

// Test 11: delete --confirm without --wait issues the DELETE and prints an
// honest, unconfirmed receipt. Production takes ~40s to actually remove the
// brand, so "deleted" must be false here — it has not been confirmed, only
// accepted — and the receipt must point the caller at how to confirm it.
func TestBrandDeleteWithConfirmIssuesDeleteAndPrintsReceipt(t *testing.T) {
	srv, methods := stubBrandDeleteServer(t, http.StatusNoContent)

	out, _, err := runBrandCmd(t, srv, "brand", "delete", "BEXMPL6", "--confirm", "--plain")
	if err != nil {
		t.Fatalf("brand delete --confirm: %v", err)
	}
	if len(*methods) != 1 || (*methods)[0] != http.MethodDelete {
		t.Fatalf("want exactly one DELETE, got %v", *methods)
	}
	got := decodeStdout(t, out)
	if got["id"] != "BEXMPL6" {
		t.Errorf("stdout = %v, want id BEXMPL6", got)
	}
	if got["deleted"] != false {
		t.Errorf("stdout = %v, want deleted false — accepted is not confirmed, and there was no --wait to confirm it", got)
	}
	if got["status"] != "accepted" {
		t.Errorf("stdout = %v, want status accepted", got)
	}
	note, _ := got["note"].(string)
	if !strings.Contains(note, "brand get") {
		t.Errorf("note = %q, want it to point at how to confirm completion", note)
	}
}

// Test 12: delete --confirm --wait, where the follow-up read 404s, exits 0
// and prints deleted:true ONLY now that the 404 actually confirmed it.
// GoneIsDone is the one place in this command set where a 404 means success
// rather than "not ready yet".
func TestBrandDeleteWaitTreats404AsSuccess(t *testing.T) {
	srv, methods := stubBrandDeleteServer(t, http.StatusNoContent)

	out, _, err := runBrandCmd(t, srv, "brand", "delete", "BEXMPL6", "--confirm", "--wait", "--timeout", "5", "--plain")
	if err != nil {
		t.Fatalf("brand delete --confirm --wait: %v", err)
	}
	if len(*methods) < 2 || (*methods)[0] != http.MethodDelete || (*methods)[1] != http.MethodGet {
		t.Fatalf("want a DELETE then at least one GET, got %v", *methods)
	}
	got := decodeStdout(t, out)
	if got["id"] != "BEXMPL6" {
		t.Errorf("stdout = %v, want id BEXMPL6", got)
	}
	if got["deleted"] != true {
		t.Errorf("stdout = %v, want deleted true — the follow-up 404 confirmed it", got)
	}
	if _, present := got["note"]; present {
		t.Errorf("stdout = %v, want no unconfirmed-delete note once the 404 confirmed completion", got)
	}
}

// Test 13: delete --confirm --wait --timeout 0, where the follow-up read
// never 404s before the deadline (matching production's real ~40s
// propagation delay), exits 5 (timeout) and must NOT claim deleted:true — a
// receipt that contradicts its own exit code is the exact bug this guards.
func TestBrandDeleteWaitTimeoutKeepsReceiptHonest(t *testing.T) {
	srv, methods := stubBrandDeleteStillExistsServer(t, http.StatusNoContent)

	out, _, err := runBrandCmd(t, srv, "brand", "delete", "BEXMPL6", "--confirm", "--wait", "--timeout", "0", "--plain")
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitTimeout {
		t.Errorf("exit code = %d, want %d (timeout)", code, cmdutil.ExitTimeout)
	}
	if len(*methods) < 2 || (*methods)[0] != http.MethodDelete || (*methods)[1] != http.MethodGet {
		t.Fatalf("want a DELETE then at least one GET, got %v", *methods)
	}
	got := decodeStdout(t, out)
	if got["id"] != "BEXMPL6" {
		t.Errorf("stdout = %v, want id BEXMPL6", got)
	}
	if got["deleted"] != false {
		t.Errorf("stdout = %v, want deleted false — the timeout means completion was never confirmed, and exit 5 must not be paired with deleted:true", got)
	}
	if got["status"] != "accepted" {
		t.Errorf("stdout = %v, want status accepted", got)
	}
	note, _ := got["note"].(string)
	if !strings.Contains(note, "brand get") {
		t.Errorf("note = %q, want it to point at how to confirm completion", note)
	}
}
