package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDoRawResponse_ExposesStatusAndHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/xml" {
			t.Errorf("Accept = %q, want application/xml", r.Header.Get("Accept"))
		}
		w.Header().Set("X-Test", "yes")
		w.WriteHeader(200)
		w.Write([]byte("<Ok/>"))
	}))
	defer srv.Close()

	c := NewXMLClient(srv.URL, nil)
	resp, err := c.DoRawResponse("GET", "/realms", nil)
	if err != nil {
		t.Fatalf("DoRawResponse() error = %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("X-Test") != "yes" {
		t.Errorf("header X-Test = %q, want yes", resp.Header.Get("X-Test"))
	}
	if string(resp.Body) != "<Ok/>" {
		t.Errorf("Body = %q, want <Ok/>", resp.Body)
	}
}

func TestDoRawResponse_FollowsSameOriginRedirectAndReportsFinalURL(t *testing.T) {
	// Mirrors the live behavior: an empty credential list 303s to ?page=1&size=500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "" {
			http.Redirect(w, r, r.URL.Path+"?page=1&size=500", http.StatusSeeOther)
			return
		}
		w.Write([]byte("<SipCredentialsResponse/>"))
	}))
	defer srv.Close()

	c := NewXMLClient(srv.URL, nil)
	resp, err := c.DoRawResponse("GET", "/realms/1/sipcredentials", nil)
	if err != nil {
		t.Fatalf("DoRawResponse() error = %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if got := resp.FinalURL.Query().Get("page"); got != "1" {
		t.Errorf("FinalURL page = %q, want 1", got)
	}
}

func TestSameOriginRedirect_RefusesOffHost(t *testing.T) {
	mk := func(raw string) *http.Request {
		u, _ := url.Parse(raw)
		return &http.Request{URL: u}
	}
	via := []*http.Request{mk("https://api.bandwidth.com/api/v2/realms")}

	if err := sameOriginRedirect(mk("https://api.bandwidth.com/api/v2/realms?page=1"), via); err != nil {
		t.Errorf("same origin redirect refused: %v", err)
	}
	for _, target := range []string{
		"https://evil.example.com/realms", // different host
		"http://api.bandwidth.com/realms", // HTTPS -> HTTP downgrade
	} {
		if err := sameOriginRedirect(mk(target), via); err == nil {
			t.Errorf("redirect to %s allowed, want refusal", target)
		}
	}
}

func TestSameOriginRedirect_HopLimit(t *testing.T) {
	u, _ := url.Parse("https://api.bandwidth.com/a")
	via := make([]*http.Request, 11)
	for i := range via {
		via[i] = &http.Request{URL: u}
	}
	err := sameOriginRedirect(&http.Request{URL: u}, via)
	if err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Errorf("err = %v, want too many redirects", err)
	}
}
