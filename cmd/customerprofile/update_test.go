package customerprofile

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// The regression lock for this whole PR: an update must not delete a field the
// CLI does not model. PUT is a full replacement, so a dropped field is nulled.
func TestUpdatePreservesUnmodeledFields(t *testing.T) {
	var putBody map[string]any
	_, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"id":"abc","name":"Acme","website":"https://acme.com",
				"version":3,"futureField":"keep me","totalCampaigns":2}}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &putBody)
		_, _ = w.Write([]byte(`{"data":{"id":"abc","version":4}}`))
	}, "update", "abc", "--name", "Acme Renamed", "--plain")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if putBody["futureField"] != "keep me" {
		t.Errorf("unmodeled field dropped from PUT — it would be nulled server-side: %#v", putBody)
	}
	if putBody["website"] != "https://acme.com" {
		t.Errorf("unchanged website dropped: %#v", putBody)
	}
	if putBody["name"] != "Acme Renamed" {
		t.Errorf("name = %v, want the new value", putBody["name"])
	}
	if putBody["version"] == nil {
		t.Error("version missing from PUT — the API rejects updates without it")
	}
	if _, present := putBody["totalCampaigns"]; present {
		t.Error("read-only totalCampaigns should be stripped before PUT")
	}
}

func TestUpdateReadsBeforeWriting(t *testing.T) {
	var methods []string
	_, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"id":"abc","name":"Acme","version":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"id":"abc","version":2}}`))
	}, "update", "abc", "--website", "https://new.example", "--plain")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(methods) != 2 || methods[0] != http.MethodGet || methods[1] != http.MethodPut {
		t.Errorf("request sequence = %v, want [GET PUT]", methods)
	}
}

// The API rejects an empty string on website ("size must be between 1 and
// 500") but accepts and applies JSON null, measured against production
// (account 9901287). The documented way to clear a field is passing the flag
// with an empty value, so that must reach the server as null, not "". This
// asserts on the JSON the server received in the PUT, not on what was passed
// in — the same standard as TestUpdatePreservesUnmodeledFields.
func TestUpdateClearFieldSendsNull(t *testing.T) {
	var putBody map[string]any
	_, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"id":"abc","name":"Acme","website":"https://acme.com","version":1}}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &putBody)
		_, _ = w.Write([]byte(`{"data":{"id":"abc","version":2}}`))
	}, "update", "abc", "--website", "", "--plain")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	val, present := putBody["website"]
	if !present {
		t.Fatal("website key missing from PUT body, want it present with a null value")
	}
	if val != nil {
		t.Errorf("website = %v, want explicit null so the API clears the field", val)
	}
}

func TestUpdateWithNoFlagsIsAnError(t *testing.T) {
	_, err := runCmd(t, nil, "update", "abc")
	if err == nil {
		t.Fatal("expected an error: update with no field flags would be a no-op round trip")
	}
	if !strings.Contains(err.Error(), "nothing to update") {
		t.Errorf("error = %q, want it to say nothing was requested", err.Error())
	}
}

// Confirmed live against production: --name "" overlays a null name onto the
// PUT body, and the API answers with a raw 400 "name must not be null". That
// must be caught locally instead — a FlagError (exit 6) with zero writes.
func TestUpdateEmptyNameExitsSixWithNoWrite(t *testing.T) {
	putCalled := false
	_, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putCalled = true
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"id":"abc","name":"Acme","version":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"id":"abc","version":2}}`))
	}, "update", "abc", "--name", "", "--plain")
	if err == nil {
		t.Fatal("expected an error when --name is cleared to empty")
	}
	if got := exitCodeOf(err); got != 6 {
		t.Errorf("exit code = %d, want 6", got)
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error = %q, want it to name the field", err.Error())
	}
	if putCalled {
		t.Error("PUT was issued despite an invalid update body — validation must happen before any write")
	}
}

// A 409 means someone else wrote between our GET and PUT. That is a conflict
// the caller can resolve by retrying, so it must exit 4, not 1.
func TestUpdateConflictExitsFour(t *testing.T) {
	_, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":{"id":"abc","name":"Acme","version":1}}`))
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"errors":[{"description":"entity has been modified by another process or user"}]}`))
	}, "update", "abc", "--name", "X", "--plain")
	if err == nil {
		t.Fatal("expected a conflict error")
	}
	if got := exitCodeOf(err); got != 4 {
		t.Errorf("exit code = %d, want 4 (conflict)", got)
	}
	if !strings.Contains(err.Error(), "retry") {
		t.Errorf("error = %q, want it to tell the caller to retry", err.Error())
	}
}
