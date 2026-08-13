package customerprofile

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
)

func newTestService(t *testing.T, h http.HandlerFunc) (*Service, func()) {
	t.Helper()
	return newTestServiceForAccount(t, "9901287", h)
}

func newTestServiceForAccount(t *testing.T, accountID string, h http.HandlerFunc) (*Service, func()) {
	t.Helper()
	srv := httptest.NewServer(h)
	return NewService(api.NewClientNoAuth(srv.URL), accountID), srv.Close
}

func TestListBuildsPathAndQuery(t *testing.T) {
	var gotPath, gotQuery string
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[],"page":{"totalElements":0}}`))
	})
	defer done()

	_, err := svc.List(10, 0, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/api/v2/accounts/9901287/customerProfiles"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if want := "limit=10"; gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
}

func TestListWithFilterBuildsQuery(t *testing.T) {
	var gotQuery string
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[],"page":{"totalElements":0}}`))
	})
	defer done()

	_, err := svc.List(25, 5, []api.Filter{{Field: "brandId", Op: api.OpEq, Value: "B0IRNU4"}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "brandId%5Beq%5D=B0IRNU4&limit=25&offset=5"; gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
}

func TestListEscapesAccountID(t *testing.T) {
	var gotPath string
	svc, done := newTestServiceForAccount(t, "99/../../etc", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	defer done()

	_, err := svc.List(0, 0, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if want := "/api/v2/accounts/99%2F..%2F..%2Fetc/customerProfiles"; gotPath != want {
		t.Errorf("escaped path = %q, want %q", gotPath, want)
	}
}

func TestGetReturnsObjectEnvelope(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"customerProfileId":"CP123"}}`))
	})
	defer done()

	env, err := svc.Get("CP123")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	obj, err := env.Object()
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	if obj["customerProfileId"] != "CP123" {
		t.Errorf("customerProfileId = %v", obj["customerProfileId"])
	}
}

func TestGetEscapesID(t *testing.T) {
	var gotPath string
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	defer done()

	_, _ = svc.Get("CP/../evil")
	if want := "/api/v2/accounts/9901287/customerProfiles/CP%2F..%2Fevil"; gotPath != want {
		t.Errorf("escaped path = %q, want %q", gotPath, want)
	}
}

// TestGetEmptyIDErrorsBeforeRequest asserts not just that Get("") errors, but
// that it does so WITHOUT making a request — the handler calls t.Fatal if
// hit, so this fails if the empty-ID guard is ever removed or short-circuited
// after the request is issued.
func TestGetEmptyIDErrorsBeforeRequest(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for an empty profile ID")
	})
	defer done()

	if _, err := svc.Get(""); err == nil {
		t.Fatal("expected an error for an empty profile ID")
	}
}

func TestServicePropagatesAPIError(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"description":"does not have access rights"}]}`))
	})
	defer done()

	_, err := svc.List(0, 0, nil)
	if err == nil {
		t.Fatal("expected an error for 403")
	}

	// Callers (e.g. band customer-profile status) need to branch on the HTTP
	// status code, which requires *api.APIError to survive this layer
	// unwrapped rather than being flattened into a plain error string. Assert
	// the type and the status, not just "an error happened" — this fails if
	// the service wraps with fmt.Errorf("%v", err) instead of "%w".
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *api.APIError (or something wrapping it)", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusForbidden)
	}
}
