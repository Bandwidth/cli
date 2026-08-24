package tendlc

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
)

// captured records what the stub server saw, so tests assert on the request
// the service actually sent rather than on a mock's expectations.
type captured struct {
	method      string
	path        string
	escapedPath string
	query       string
	body        map[string]any
}

func stubService(t *testing.T, status int, respBody string, got *captured) *Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.escapedPath = r.URL.EscapedPath()
		got.query = r.URL.RawQuery
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &got.body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if respBody != "" {
			_, _ = io.WriteString(w, respBody)
		}
	}))
	t.Cleanup(srv.Close)
	return NewService(api.NewClientNoAuth(srv.URL), "9901287")
}

func TestCreateBrandPostsToBrandsPath(t *testing.T) {
	var got captured
	s := stubService(t, 202, `{"data":{"bandwidthId":"WABC123"}}`, &got)

	env, err := s.CreateBrand(map[string]any{"displayName": "Acme"})
	if err != nil {
		t.Fatalf("CreateBrand: %v", err)
	}
	if got.method != "POST" {
		t.Errorf("method = %q, want POST", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/brands"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	if got.body["displayName"] != "Acme" {
		t.Errorf("body displayName = %v, want Acme", got.body["displayName"])
	}
	obj, err := env.Object()
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	if obj["bandwidthId"] != "WABC123" {
		t.Errorf("bandwidthId = %v, want WABC123", obj["bandwidthId"])
	}
}

func TestUpdateBrandPutsToBrandPath(t *testing.T) {
	var got captured
	s := stubService(t, 202, `{"data":{"bandwidthId":"WABC123"}}`, &got)

	if _, err := s.UpdateBrand("BEXMPL6", map[string]any{"displayName": "Acme"}); err != nil {
		t.Fatalf("UpdateBrand: %v", err)
	}
	if got.method != "PUT" {
		t.Errorf("method = %q, want PUT", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/brands/BEXMPL6"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
}

func TestDeleteBrandUsesDelete(t *testing.T) {
	var got captured
	s := stubService(t, 202, "", &got)

	if err := s.DeleteBrand("WET8JUY8H0"); err != nil {
		t.Fatalf("DeleteBrand: %v", err)
	}
	if got.method != "DELETE" {
		t.Errorf("method = %q, want DELETE", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/brands/WET8JUY8H0"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
}

func TestReverifyAndResend2FAPostToIdentityPaths(t *testing.T) {
	tests := []struct {
		name string
		call func(*Service) error
		want string
	}{
		{"reverify", func(s *Service) error { return s.ReverifyBrand("BEXMPL6") },
			"/api/v2/accounts/9901287/tendlc/brands/BEXMPL6/identity/reverify"},
		{"resend2fa", func(s *Service) error { return s.Resend2FA("BEXMPL6") },
			"/api/v2/accounts/9901287/tendlc/brands/BEXMPL6/identity/resend2faEmail"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got captured
			// 204 with an empty body: these endpoints return no content, so the
			// service must not try to parse an envelope out of nothing.
			s := stubService(t, 204, "", &got)
			if err := tt.call(s); err != nil {
				t.Fatalf("call: %v", err)
			}
			if got.method != "POST" {
				t.Errorf("method = %q, want POST", got.method)
			}
			if got.path != tt.want {
				t.Errorf("path = %q, want %q", got.path, tt.want)
			}
		})
	}
}

func TestBrandHistoryEncodesPagination(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":[],"page":{"totalElements":0}}`, &got)

	if _, err := s.BrandHistory("BEXMPL6", 10, 20); err != nil {
		t.Fatalf("BrandHistory: %v", err)
	}
	if want := "/api/v2/accounts/9901287/tendlc/brands/BEXMPL6/history"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	if got.query != "limit=10&offset=20" {
		t.Errorf("query = %q, want limit=10&offset=20", got.query)
	}
}

func TestListVettingsEncodesPagination(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":[],"page":{"totalElements":0}}`, &got)

	if _, err := s.ListVettings("BEXMPL6", 10, 0); err != nil {
		t.Fatalf("ListVettings: %v", err)
	}
	if want := "/api/v2/accounts/9901287/tendlc/brands/BEXMPL6/vettings"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
}

func TestRequestVettingPostsBody(t *testing.T) {
	var got captured
	s := stubService(t, 202, `{"data":{"bandwidthId":"WV123"}}`, &got)

	body := map[string]any{"evpId": "AEGIS", "vettingClass": "STANDARD"}
	if _, err := s.RequestVetting("BEXMPL6", body); err != nil {
		t.Fatalf("RequestVetting: %v", err)
	}
	if got.method != "POST" {
		t.Errorf("method = %q, want POST", got.method)
	}
	if got.body["evpId"] != "AEGIS" || got.body["vettingClass"] != "STANDARD" {
		t.Errorf("body = %v, want evpId/vettingClass preserved", got.body)
	}
}

func TestImportVettingPutsToVettingPath(t *testing.T) {
	var got captured
	s := stubService(t, 202, `{"data":{"bandwidthId":"WV123"}}`, &got)

	if _, err := s.ImportVetting("BEXMPL6", "978de74a-7191", map[string]any{"evpId": "AEGIS"}); err != nil {
		t.Fatalf("ImportVetting: %v", err)
	}
	if got.method != "PUT" {
		t.Errorf("method = %q, want PUT", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/brands/BEXMPL6/vettings/978de74a-7191"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
}

// Every method that takes an ID must reject an empty one before making a
// request. Without this a caller with an unset variable silently hits the
// collection endpoint — DELETE on /brands rather than /brands/{id}.
func TestEmptyIDsRejectedWithoutRequest(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":{}}`, &got)

	calls := map[string]func() error{
		"UpdateBrand":   func() error { _, err := s.UpdateBrand("", map[string]any{}); return err },
		"DeleteBrand":   func() error { return s.DeleteBrand("") },
		"ReverifyBrand": func() error { return s.ReverifyBrand("") },
		"Resend2FA":     func() error { return s.Resend2FA("") },
		"BrandHistory":  func() error { _, err := s.BrandHistory("", 10, 0); return err },
		"ListVettings":  func() error { _, err := s.ListVettings("", 10, 0); return err },
		"RequestVetting": func() error {
			_, err := s.RequestVetting("", map[string]any{})
			return err
		},
		"ImportVettingNoBrand": func() error {
			_, err := s.ImportVetting("", "v1", map[string]any{})
			return err
		},
		"ImportVettingNoVetting": func() error {
			_, err := s.ImportVetting("B1", "", map[string]any{})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			got = captured{}
			if err := call(); err == nil {
				t.Fatal("want an error for an empty ID, got nil")
			}
			if got.method != "" {
				t.Errorf("a request was made (%s %s); want none", got.method, got.path)
			}
		})
	}
}

// IDs go into the path, so a value containing a slash or a space must be
// escaped rather than silently changing which endpoint is called.
func TestBrandIDIsPathEscaped(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":[],"page":{"totalElements":0}}`, &got)

	if _, err := s.BrandHistory("a/b c", 10, 0); err != nil {
		t.Fatalf("BrandHistory: %v", err)
	}
	if want := "/api/v2/accounts/9901287/tendlc/brands/a%2Fb%20c/history"; got.escapedPath != want {
		t.Errorf("escaped path = %q, want %q", got.escapedPath, want)
	}
}

// A vetting ID is an externally-supplied provider value, not one Bandwidth
// assigns — so it must be escaped in the URL just like a brand ID.
// TestImportVettingPutsToVettingPath above asserts on got.path, which
// net/url DECODES, so it would pass identically whether
// url.PathEscape(vettingID) was called or not (this is the same "regression
// guard that guards nothing" class Task 1's brandPath test hit — see the
// escapedPath field's own history). This test asserts on got.escapedPath
// instead, so it actually catches the escape being dropped.
func TestVettingIDIsPathEscaped(t *testing.T) {
	var got captured
	s := stubService(t, 202, `{"data":{"bandwidthId":"WV123"}}`, &got)

	if _, err := s.ImportVetting("BEXMPL6", "v/1 2", map[string]any{"evpId": "AEGIS"}); err != nil {
		t.Fatalf("ImportVetting: %v", err)
	}
	if want := "/api/v2/accounts/9901287/tendlc/brands/BEXMPL6/vettings/v%2F1%202"; got.escapedPath != want {
		t.Errorf("escaped path = %q, want %q", got.escapedPath, want)
	}
}
