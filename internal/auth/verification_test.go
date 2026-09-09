package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTokenVerificationRejectsRevokedSecretDespiteCachedToken(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			fmt.Fprint(w, `{"access_token":"cached-token","expires_in":3600}`)
			return
		}
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":"invalid_client","error_description":"secret-must-not-leak"}`)
	}))
	defer srv.Close()
	tm := NewTokenManager("id", "secret-must-not-leak", srv.URL)
	tm.ProfileName = "admin"
	if _, err := tm.GetToken(); err != nil {
		t.Fatal(err)
	}
	_, _, err := tm.Verify(context.Background())
	var tokenErr *TokenError
	if !errors.As(err, &tokenErr) || !tokenErr.Rejected() || tokenErr.Code != "invalid_client" {
		t.Fatalf("expected rejected credentials, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("requests = %d, want fresh verification", calls)
	}
	if strings.Contains(err.Error(), "secret-must-not-leak") || !strings.Contains(err.Error(), "band auth login --profile admin") {
		t.Fatalf("unsafe or unactionable error: %v", err)
	}
}

func TestTokenContextCancelsExchange(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tm := NewTokenManager("id", "secret", srv.URL)
	done := make(chan error, 1)
	go func() { _, err := tm.GetTokenContext(ctx); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("token exchange did not cancel")
	}
}

func TestTokenErrorDropsUnknownServerText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":"secret-must-not-leak","error_description":"secret-must-not-leak"}`)
	}))
	defer srv.Close()
	_, err := NewTokenManager("id", "secret", srv.URL).GetToken()
	if err == nil || strings.Contains(err.Error(), "secret-must-not-leak") {
		t.Fatalf("error = %v", err)
	}
}
