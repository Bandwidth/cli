package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/auth"
)


func TestClient_Get(t *testing.T) {
	type response struct {
		Name string `json:"name"`
	}

	// Token server
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-token",
			"expires_in":   3600,
			"token_type":   "bearer",
		})
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			t.Error("expected Authorization header to be set")
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			t.Errorf("expected Bearer token, got %q", authHeader)
		}
		if r.Header.Get("User-Agent") != userAgent() {
			t.Errorf("User-Agent = %q, want %q", r.Header.Get("User-Agent"), userAgent())
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response{Name: "test"})
	}))
	defer srv.Close()

	tm := auth.NewTokenManager("client-id", "client-secret", tokenSrv.URL)
	client := NewClient(srv.URL, tm)

	var got response
	if err := client.Get("/", &got); err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("Name = %q, want %q", got.Name, "test")
	}
}

func TestClient_Post(t *testing.T) {
	type request struct {
		Value string `json:"value"`
	}
	type response struct {
		Result string `json:"result"`
	}

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		if req.Value != "hello" {
			t.Errorf("Value = %q, want %q", req.Value, "hello")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response{Result: "ok"})
	}))
	defer srv.Close()

	tm := auth.NewTokenManager("client-id", "client-secret", tokenSrv.URL)
	client := NewClient(srv.URL, tm)

	var got response
	if err := client.Post("/", request{Value: "hello"}, &got); err != nil {
		t.Fatalf("Post() error: %v", err)
	}
	if got.Result != "ok" {
		t.Errorf("Result = %q, want %q", got.Result, "ok")
	}
}

func TestClient_ErrorResponse(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	tm := auth.NewTokenManager("client-id", "client-secret", tokenSrv.URL)
	client := NewClient(srv.URL, tm)

	var got struct{}
	err := client.Get("/missing", &got)
	if err == nil {
		t.Fatal("expected error for non-2xx response, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusNotFound)
	}
}

func TestClient_Put(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}
	type response struct {
		Updated bool `json:"updated"`
	}

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response{Updated: true})
	}))
	defer srv.Close()

	tm := auth.NewTokenManager("client-id", "client-secret", tokenSrv.URL)
	client := NewClient(srv.URL, tm)

	var got response
	if err := client.Put("/", request{Name: "test"}, &got); err != nil {
		t.Fatalf("Put() error: %v", err)
	}
	if !got.Updated {
		t.Error("expected Updated = true")
	}
}

func TestClient_Delete(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	tm := auth.NewTokenManager("client-id", "client-secret", tokenSrv.URL)
	client := NewClient(srv.URL, tm)

	// nil result is valid for 204 No Content
	if err := client.Delete("/", nil); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestClient_GetRaw(t *testing.T) {
	payload := []byte("raw binary data")

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()

	tm := auth.NewTokenManager("client-id", "client-secret", tokenSrv.URL)
	client := NewClient(srv.URL, tm)

	got, err := client.GetRaw("/")
	if err != nil {
		t.Fatalf("GetRaw() error: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("GetRaw() = %q, want %q", got, payload)
	}
}

func TestClient_NoAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("expected no Authorization header for NoAuth client")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(struct{}{})
	}))
	defer srv.Close()

	client := NewClientNoAuth(srv.URL)

	var got struct{}
	if err := client.Get("/", &got); err != nil {
		t.Fatalf("Get() error: %v", err)
	}
}

func TestAPIError_Error(t *testing.T) {
	e := &APIError{StatusCode: 422, Body: "unprocessable entity"}
	msg := e.Error()
	if msg == "" {
		t.Error("APIError.Error() returned empty string")
	}
}

func TestAPIError_Error_EmptyBodyFallsBackToStatusText(t *testing.T) {
	// Some endpoints (e.g. /tns) return a 403 with no body. The raw message
	// was "API error 403: " — ending with a colon and nothing useful. Fall
	// back to the HTTP status text instead.
	e := &APIError{StatusCode: 403, Body: ""}
	msg := e.Error()
	if !strings.Contains(msg, "Forbidden") {
		t.Errorf("expected fallback to status text, got %q", msg)
	}
	if strings.HasSuffix(msg, ": ") {
		t.Errorf("message should not end with an empty colon suffix, got %q", msg)
	}
}

func TestAPIError_Error_WhitespaceBodyFallsBack(t *testing.T) {
	// A body that is only whitespace is as useless as an empty one.
	e := &APIError{StatusCode: 401, Body: "   \n\t"}
	msg := e.Error()
	if !strings.Contains(msg, "Unauthorized") {
		t.Errorf("expected fallback to status text, got %q", msg)
	}
}

func TestAPIError_Error_UnknownStatusCode(t *testing.T) {
	// Unknown status codes (no http.StatusText mapping) should still produce
	// a readable error rather than a trailing colon.
	e := &APIError{StatusCode: 999, Body: ""}
	msg := e.Error()
	if !strings.Contains(msg, "empty response body") {
		t.Errorf("expected empty-body marker, got %q", msg)
	}
}

// ---- XML client tests ----

func TestXMLClient_Post(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the client sent XML.
		ct := r.Header.Get("Content-Type")
		if ct != "application/xml" {
			t.Errorf("Content-Type = %q, want application/xml", ct)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// Verify body contains expected XML.
		body := make([]byte, 512)
		n, _ := r.Body.Read(body)
		bodyStr := string(body[:n])
		if !strings.Contains(bodyStr, "<SipPeer>") {
			t.Errorf("expected XML root element <SipPeer>, got:\n%s", bodyStr)
		}
		if !strings.Contains(bodyStr, "<PeerName>Test Location</PeerName>") {
			t.Errorf("expected PeerName element, got:\n%s", bodyStr)
		}

		// Respond with XML.
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<SipPeerResponse>
  <SipPeer>
    <PeerName>Test Location</PeerName>
    <PeerId>12345</PeerId>
  </SipPeer>
</SipPeerResponse>`))
	}))
	defer srv.Close()

	tm := auth.NewTokenManager("client-id", "client-secret", tokenSrv.URL)
	client := NewXMLClient(srv.URL, tm)

	body := XMLBody{
		RootElement: "SipPeer",
		Data: map[string]interface{}{
			"PeerName": "Test Location",
		},
	}

	var result interface{}
	if err := client.Post("/sippeers", body, &result); err != nil {
		t.Fatalf("Post() error: %v", err)
	}

	// Result should be a map decoded from XML.
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if _, ok := m["SipPeerResponse"]; !ok {
		t.Errorf("expected SipPeerResponse key in result, got: %v", m)
	}
}

func TestXMLClient_Get(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<Site>
  <SiteId>99</SiteId>
  <Name>My Site</Name>
</Site>`))
	}))
	defer srv.Close()

	tm := auth.NewTokenManager("client-id", "client-secret", tokenSrv.URL)
	client := NewXMLClient(srv.URL, tm)

	var result interface{}
	if err := client.Get("/sites/99", &result); err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if _, ok := m["Site"]; !ok {
		t.Errorf("expected Site key in result, got: %v", m)
	}
}

func TestClient_PutRaw(t *testing.T) {
	payload := []byte("binary image data here")

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if ct != "image/png" {
			t.Errorf("Content-Type = %q, want image/png", ct)
		}
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			t.Errorf("expected Bearer token, got %q", authHeader)
		}
		body := make([]byte, 1024)
		n, _ := r.Body.Read(body)
		if string(body[:n]) != string(payload) {
			t.Errorf("body = %q, want %q", body[:n], payload)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tm := auth.NewTokenManager("client-id", "client-secret", tokenSrv.URL)
	client := NewClient(srv.URL, tm)

	if err := client.PutRaw("/media/test.png", payload, "image/png"); err != nil {
		t.Fatalf("PutRaw() error: %v", err)
	}
}

func TestClient_PutRaw_Error(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
	}))
	defer srv.Close()

	tm := auth.NewTokenManager("client-id", "client-secret", tokenSrv.URL)
	client := NewClient(srv.URL, tm)

	err := client.PutRaw("/media/test.xyz", []byte("data"), "application/octet-stream")
	if err == nil {
		t.Fatal("expected error for 415 response, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusUnsupportedMediaType)
	}
}

func TestXMLClient_NonXMLBodyReturnsError(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tm := auth.NewTokenManager("client-id", "client-secret", tokenSrv.URL)
	client := NewXMLClient(srv.URL, tm)

	// Passing a plain map (not XMLBody) to an XML client should return an error.
	err := client.Post("/test", map[string]string{"key": "val"}, nil)
	if err == nil {
		t.Fatal("expected error when passing non-XMLBody to XML client, got nil")
	}
	if !strings.Contains(err.Error(), "XMLBody") {
		t.Errorf("expected error mentioning XMLBody, got: %v", err)
	}
}
