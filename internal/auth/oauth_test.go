package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTokenServer(t *testing.T, token string, expiresIn int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method and Content-Type
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}
		// Verify Authorization header contains Basic auth
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Basic ") {
			t.Errorf("Authorization = %q, want Basic ...", authHeader)
		}
		// Verify grant_type in body
		if err := r.ParseForm(); err != nil {
			t.Errorf("parsing form: %v", err)
		}
		if gt := r.FormValue("grant_type"); gt != "client_credentials" {
			t.Errorf("grant_type = %q, want client_credentials", gt)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": token,
			"expires_in":   expiresIn,
			"token_type":   "bearer",
		})
	}))
}

func TestTokenManager_GetToken(t *testing.T) {
	srv := newTokenServer(t, "my-access-token", 3600)
	defer srv.Close()

	tm := NewTokenManager("client-id", "client-secret", srv.URL)
	token, err := tm.GetToken()
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if token != "my-access-token" {
		t.Errorf("token = %q, want %q", token, "my-access-token")
	}
}

func TestTokenManager_CachesToken(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "cached-token",
			"expires_in":   3600,
			"token_type":   "bearer",
		})
	}))
	defer srv.Close()

	tm := NewTokenManager("client-id", "client-secret", srv.URL)

	// Call GetToken twice — should only hit the server once.
	if _, err := tm.GetToken(); err != nil {
		t.Fatalf("first GetToken() error: %v", err)
	}
	if _, err := tm.GetToken(); err != nil {
		t.Fatalf("second GetToken() error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("token server called %d times, want 1 (cached after first call)", callCount)
	}
}

func TestTokenManager_RefreshesExpiredToken(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "refreshed-token",
			"expires_in":   3600,
			"token_type":   "bearer",
		})
	}))
	defer srv.Close()

	tm := NewTokenManager("client-id", "client-secret", srv.URL)

	// Seed the manager with an already-expired token.
	tm.token = "old-token"
	tm.expiresAt = time.Now().Add(-10 * time.Second)

	token, err := tm.GetToken()
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if token != "refreshed-token" {
		t.Errorf("token = %q, want %q", token, "refreshed-token")
	}
	if callCount != 1 {
		t.Errorf("token server called %d times, want 1", callCount)
	}
}

func TestTokenManager_RefreshesTokenNearExpiry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "new-token",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	tm := NewTokenManager("client-id", "client-secret", srv.URL)

	// Seed with a token that expires in 30 seconds (within the 1-minute buffer).
	tm.token = "nearly-expired"
	tm.expiresAt = time.Now().Add(30 * time.Second)

	token, err := tm.GetToken()
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if token != "new-token" {
		t.Errorf("expected refresh; got %q, want %q", token, "new-token")
	}
}

func TestTokenManager_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	tm := NewTokenManager("bad-id", "bad-secret", srv.URL)
	_, err := tm.GetToken()
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
}

func TestTokenManager_ThreadSafe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "concurrent-token",
			"expires_in":   3600,
		})
	}))
	defer srv.Close()

	tm := NewTokenManager("client-id", "client-secret", srv.URL)

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := tm.GetToken(); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent GetToken() error: %v", err)
	}
}
