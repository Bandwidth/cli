package customerprofile

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
)

type captured struct {
	method string
	path   string
	body   map[string]any
}

func newCapturingService(t *testing.T, status int, respBody string) (*Service, *captured, func()) {
	t.Helper()
	cap := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.method = r.Method
		cap.path = r.URL.EscapedPath()
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &cap.body)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	return NewService(api.NewClientNoAuth(srv.URL), "9901287"), cap, srv.Close
}

func TestCreatePostsToCollection(t *testing.T) {
	svc, cap, done := newCapturingService(t, 200, `{"data":{"id":"abc","version":0}}`)
	defer done()

	env, err := svc.Create(map[string]any{"name": "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if cap.method != http.MethodPost {
		t.Errorf("method = %s, want POST", cap.method)
	}
	if want := "/api/v2/accounts/9901287/customerProfiles"; cap.path != want {
		t.Errorf("path = %q, want %q", cap.path, want)
	}
	if cap.body["name"] != "Acme" {
		t.Errorf("body name = %v, want Acme", cap.body["name"])
	}
	obj, err := env.Object()
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	if obj["id"] != "abc" {
		t.Errorf("id = %v", obj["id"])
	}
}

func TestUpdatePutsToResourceAndSendsBodyVerbatim(t *testing.T) {
	svc, cap, done := newCapturingService(t, 200, `{"data":{"id":"abc","version":3}}`)
	defer done()

	// An unknown field must reach the wire untouched — that is the whole
	// point of building the payload from the read map.
	body := map[string]any{"name": "Acme", "version": 2, "somethingWeNeverModeled": "keep me"}
	if _, err := svc.Update("abc", body); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if cap.method != http.MethodPut {
		t.Errorf("method = %s, want PUT", cap.method)
	}
	if want := "/api/v2/accounts/9901287/customerProfiles/abc"; cap.path != want {
		t.Errorf("path = %q, want %q", cap.path, want)
	}
	if cap.body["somethingWeNeverModeled"] != "keep me" {
		t.Errorf("unknown field was dropped: %#v", cap.body)
	}
}

func TestDeleteHitsResource(t *testing.T) {
	svc, cap, done := newCapturingService(t, 204, ``)
	defer done()

	if err := svc.Delete("abc"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if cap.method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", cap.method)
	}
	if want := "/api/v2/accounts/9901287/customerProfiles/abc"; cap.path != want {
		t.Errorf("path = %q, want %q", cap.path, want)
	}
}

func TestHistoryPathsAndPaging(t *testing.T) {
	svc, cap, done := newCapturingService(t, 200, `{"data":[],"page":{"totalElements":0}}`)
	defer done()

	if _, err := svc.History("abc", 10, 20); err != nil {
		t.Fatalf("History: %v", err)
	}
	if want := "/api/v2/accounts/9901287/customerProfiles/abc/history"; cap.path != want {
		t.Errorf("path = %q, want %q", cap.path, want)
	}
}

func TestHistoryVersionPath(t *testing.T) {
	svc, cap, done := newCapturingService(t, 200, `{"data":{"version":2}}`)
	defer done()

	if _, err := svc.HistoryVersion("abc", "2"); err != nil {
		t.Fatalf("HistoryVersion: %v", err)
	}
	if want := "/api/v2/accounts/9901287/customerProfiles/abc/history/2"; cap.path != want {
		t.Errorf("path = %q, want %q", cap.path, want)
	}
}

func TestWriteMethodsRequireAnID(t *testing.T) {
	svc, _, done := newCapturingService(t, 200, `{}`)
	defer done()

	if _, err := svc.Update("", map[string]any{}); err == nil {
		t.Error("Update(\"\") should error before making a request")
	}
	if err := svc.Delete(""); err == nil {
		t.Error("Delete(\"\") should error before making a request")
	}
	if _, err := svc.HistoryVersion("abc", ""); err == nil {
		t.Error("HistoryVersion with empty version should error")
	}
}

func TestWriteMethodsPropagateAPIErrorType(t *testing.T) {
	svc, _, done := newCapturingService(t, 409,
		`{"errors":[{"description":"entity has been modified by another process or user"}]}`)
	defer done()

	_, err := svc.Update("abc", map[string]any{"name": "x"})
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *api.APIError to survive", err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("StatusCode = %d, want 409", apiErr.StatusCode)
	}
}
