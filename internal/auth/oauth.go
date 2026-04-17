package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenManager handles fetching and caching OAuth2 client credentials tokens.
type TokenManager struct {
	ClientID     string
	ClientSecret string
	TokenURL     string

	token     string
	expiresAt time.Time
	mu        sync.Mutex
}

// NewTokenManager creates a TokenManager for the given client credentials and token URL.
func NewTokenManager(clientID, clientSecret, tokenURL string) *TokenManager {
	return &TokenManager{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
	}
}

// tokenResponse is the JSON body returned by the OAuth2 token endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// GetToken returns a valid Bearer token, fetching a new one if the cached token
// is missing or within 1 minute of expiry.
func (tm *TokenManager) GetToken() (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Return cached token if it has more than 1 minute remaining.
	if tm.token != "" && time.Now().Add(time.Minute).Before(tm.expiresAt) {
		return tm.token, nil
	}

	return tm.fetchToken()
}

// fetchToken performs the token exchange. Caller must hold tm.mu.
func (tm *TokenManager) fetchToken() (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequest(http.MethodPost, tm.TokenURL+"/api/v1/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+EncodeBasicAuth(tm.ClientID, tm.ClientSecret))
	req.Header.Set("User-Agent", "band-cli/0.1.0")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}

	if tr.AccessToken == "" {
		return "", fmt.Errorf("token response missing access_token")
	}

	tm.token = tr.AccessToken
	if tr.ExpiresIn > 0 {
		tm.expiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	} else {
		// Default to 1 hour if no expiry is given.
		tm.expiresAt = time.Now().Add(time.Hour)
	}

	return tm.token, nil
}
