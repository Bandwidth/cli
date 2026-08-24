package tendlc

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

func TestListBrandsBuildsPathAndQuery(t *testing.T) {
	var gotPath, gotQuery string
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[],"page":{"totalElements":0}}`))
	})
	defer done()

	_, err := svc.ListBrands(25, 0, []api.Filter{{Field: "brandType", Op: api.OpEq, Value: "PUBLIC_PROFIT"}})
	if err != nil {
		t.Fatalf("ListBrands: %v", err)
	}
	if want := "/api/v2/accounts/9901287/tendlc/brands"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if want := "brandType%5Beq%5D=PUBLIC_PROFIT&limit=25"; gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
}

func TestGetBrandReturnsObjectEnvelope(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"brandId":"BEXMPL8"}}`))
	})
	defer done()

	env, err := svc.GetBrand("BEXMPL8")
	if err != nil {
		t.Fatalf("GetBrand: %v", err)
	}
	obj, err := env.Object()
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	if obj["brandId"] != "BEXMPL8" {
		t.Errorf("brandId = %v", obj["brandId"])
	}
}

func TestGetBrandEscapesID(t *testing.T) {
	var gotPath string
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	defer done()

	_, _ = svc.GetBrand("B/../evil")
	if want := "/api/v2/accounts/9901287/tendlc/brands/B%2F..%2Fevil"; gotPath != want {
		t.Errorf("escaped path = %q, want %q", gotPath, want)
	}
}

func TestListBrandsEscapesAccountID(t *testing.T) {
	var gotPath string
	svc, done := newTestServiceForAccount(t, "99/../../etc", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	defer done()

	_, err := svc.ListBrands(0, 0, nil)
	if err != nil {
		t.Fatalf("ListBrands: %v", err)
	}
	if want := "/api/v2/accounts/99%2F..%2F..%2Fetc/tendlc/brands"; gotPath != want {
		t.Errorf("escaped path = %q, want %q", gotPath, want)
	}
}

func TestServicePropagatesAPIError(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"description":"does not have access rights"}]}`))
	})
	defer done()

	_, err := svc.ListBrands(0, 0, nil)
	if err == nil {
		t.Fatal("expected an error for 403")
	}

	// band tendlc status (Task 9) distinguishes "no Registration Center
	// access" (403, a definite answer) from a transport failure by doing
	// errors.As(err, &apiErr) and branching on apiErr.StatusCode. That
	// contract only holds if *api.APIError survives this layer unwrapped
	// and untyped-away — assert both, not just "an error happened".
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *api.APIError (or something wrapping it)", err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusForbidden)
	}
}

func TestListCampaignsBuildsPathAndQuery(t *testing.T) {
	var gotPath, gotQuery string
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[],"page":{"totalElements":0}}`))
	})
	defer done()

	_, err := svc.ListCampaigns(10, 5, nil)
	if err != nil {
		t.Fatalf("ListCampaigns: %v", err)
	}
	if want := "/api/v2/accounts/9901287/tendlc/campaigns"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if want := "limit=10&offset=5"; gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
}

func TestGetCampaignEscapesID(t *testing.T) {
	var gotPath string
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"data":{"campaignId":"C123"}}`))
	})
	defer done()

	env, err := svc.GetCampaign("C/../evil")
	if err != nil {
		t.Fatalf("GetCampaign: %v", err)
	}
	if want := "/api/v2/accounts/9901287/tendlc/campaigns/C%2F..%2Fevil"; gotPath != want {
		t.Errorf("escaped path = %q, want %q", gotPath, want)
	}
	obj, err := env.Object()
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	if obj["campaignId"] != "C123" {
		t.Errorf("campaignId = %v", obj["campaignId"])
	}
}

func TestGetBrandEmptyIDErrors(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for an empty brand ID")
	})
	defer done()

	if _, err := svc.GetBrand(""); err == nil {
		t.Fatal("expected an error for an empty brand ID")
	}
}

func TestGetCampaignEmptyIDErrors(t *testing.T) {
	svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for an empty campaign ID")
	})
	defer done()

	if _, err := svc.GetCampaign(""); err == nil {
		t.Fatal("expected an error for an empty campaign ID")
	}
}
