package customerprofile

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDeleteRequiresConfirm(t *testing.T) {
	_, err := runCmd(t, nil, "delete", "abc")
	if err == nil {
		t.Fatal("expected an error without --confirm")
	}
	if !strings.Contains(err.Error(), "--confirm") {
		t.Errorf("error = %q, want it to name the flag", err.Error())
	}
	if got := exitCodeOf(err); got != 6 {
		t.Errorf("exit code = %d, want 6 — this is a usage error with no request made", got)
	}
}

// The gate must not depend on a TTY: an agent and a human get the same contract.
func TestDeleteConfirmGateIsFlagOnlyNotTTY(t *testing.T) {
	_, err := runCmd(t, nil, "delete", "abc", "--plain")
	if err == nil {
		t.Fatal("expected --confirm to be required under --plain too")
	}
}

func TestDeleteWithConfirmEmitsDeletedReceipt(t *testing.T) {
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}, "delete", "abc", "--confirm", "--plain")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q", out)
	}
	if got["deleted"] != true {
		t.Errorf(`receipt = %v, want deleted:true — this is a synchronous 204, not an async accept`, got)
	}
	if _, present := got["accepted"]; present {
		t.Error(`receipt must not say "accepted": the delete completed, it was not queued`)
	}
	if got["id"] != "abc" {
		t.Errorf("receipt id = %v, want the profile ID", got["id"])
	}
}

func TestRestoreSendsSoftDeletedNotDeleted(t *testing.T) {
	var putBody map[string]any
	_, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"id":"abc","name":"Acme","version":3,"softDeleted":true}}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &putBody)
		_, _ = w.Write([]byte(`{"data":{"id":"abc","version":4,"softDeleted":false}}`))
	}, "restore", "abc", "--plain")
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if putBody["softDeleted"] != false {
		t.Errorf("PUT body softDeleted = %v, want false", putBody["softDeleted"])
	}
	if _, present := putBody["deleted"]; present {
		t.Error(`must not send "deleted": the documented form returns 404 "Customer profile not found"`)
	}
}

func TestRestoreDoesNotRequireConfirm(t *testing.T) {
	_, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"id":"abc","name":"Acme","version":1,"softDeleted":true}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"id":"abc","name":"Acme","version":2,"softDeleted":false}}`))
	}, "restore", "abc", "--plain")
	if err != nil {
		t.Fatalf("restore should not need --confirm — it is not destructive: %v", err)
	}
}

// TestDeleteConfirmMakesNoRequest proves the --confirm gate is a client-side
// short-circuit, not merely an error surfaced after a request. A stub that
// records whether it was hit lets this test fail if the gate is ever moved
// after the HTTP call (e.g. reordered to check the response first).
func TestDeleteConfirmMakesNoRequest(t *testing.T) {
	hit := false
	_, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusNoContent)
	}, "delete", "abc")
	if err == nil {
		t.Fatal("expected an error without --confirm")
	}
	if hit {
		t.Error("server was hit without --confirm — the gate must short-circuit before any HTTP request")
	}
}
