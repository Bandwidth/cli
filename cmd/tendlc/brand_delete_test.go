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

// Test 10: delete without --confirm exits 6 and makes ZERO HTTP requests —
// runBrandCmd's `service` seam Fatals if it is ever invoked with a nil
// server, so this fails loudly (not silently) if the confirm gate ever moves
// after the service/DELETE call.
func TestBrandDeleteWithoutConfirmMakesNoRequests(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "brand", "delete", "BGJR2BA")
	if err == nil {
		t.Fatal("want an error when --confirm is missing")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "cannot be undone") {
		t.Errorf("error = %q, want it to say the delete cannot be undone", err.Error())
	}
	if !strings.Contains(err.Error(), "customer profile") {
		t.Errorf("error = %q, want it to mention the associated customer profile", err.Error())
	}
}

// Test 11: delete --confirm issues the DELETE and prints the receipt.
func TestBrandDeleteWithConfirmIssuesDeleteAndPrintsReceipt(t *testing.T) {
	srv, methods := stubBrandDeleteServer(t, http.StatusNoContent)

	out, _, err := runBrandCmd(t, srv, "brand", "delete", "BGJR2BA", "--confirm", "--plain")
	if err != nil {
		t.Fatalf("brand delete --confirm: %v", err)
	}
	if len(*methods) != 1 || (*methods)[0] != http.MethodDelete {
		t.Fatalf("want exactly one DELETE, got %v", *methods)
	}
	got := decodeStdout(t, out)
	if got["id"] != "BGJR2BA" {
		t.Errorf("stdout = %v, want id BGJR2BA", got)
	}
	if got["deleted"] != true {
		t.Errorf("stdout = %v, want deleted true", got)
	}
	if got["status"] != "accepted" {
		t.Errorf("stdout = %v, want status accepted", got)
	}
}

// Test 12: delete --confirm --wait, where the follow-up read 404s, exits 0
// and still prints the receipt. GoneIsDone is the one place in this command
// set where a 404 means success rather than "not ready yet".
func TestBrandDeleteWaitTreats404AsSuccess(t *testing.T) {
	srv, methods := stubBrandDeleteServer(t, http.StatusNoContent)

	out, _, err := runBrandCmd(t, srv, "brand", "delete", "BGJR2BA", "--confirm", "--wait", "--timeout", "5", "--plain")
	if err != nil {
		t.Fatalf("brand delete --confirm --wait: %v", err)
	}
	if len(*methods) < 2 || (*methods)[0] != http.MethodDelete || (*methods)[1] != http.MethodGet {
		t.Fatalf("want a DELETE then at least one GET, got %v", *methods)
	}
	got := decodeStdout(t, out)
	if got["id"] != "BGJR2BA" {
		t.Errorf("stdout = %v, want id BGJR2BA", got)
	}
	if got["deleted"] != true {
		t.Errorf("stdout = %v, want deleted true", got)
	}
}
