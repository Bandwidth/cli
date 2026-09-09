package auth

import (
	"fmt"

	"github.com/alessio/shellescape"
)

// CredentialError means no usable stored credentials were available.
type CredentialError struct {
	Reason  string
	Profile string
}

func (e *CredentialError) Error() string {
	return fmt.Sprintf("credentials unavailable for profile %q (%s).\nRun: %s", e.Profile, e.Reason, loginCommand(e.Profile))
}

// TokenError retains only the HTTP status and a recognized OAuth error code.
// Never retain the response body: proxies and providers may echo secrets.
type TokenError struct {
	StatusCode int
	Code       string
	Profile    string
}

func (e *TokenError) Rejected() bool {
	return e.StatusCode == 401 || (e.StatusCode == 400 && e.Code == "invalid_client")
}

func (e *TokenError) Error() string {
	if e.Rejected() {
		return fmt.Sprintf("credentials were rejected for profile %q (%s); the client ID or secret is invalid or has been revoked.\nRun: %s", e.Profile, e.Code, loginCommand(e.Profile))
	}
	return fmt.Sprintf("token exchange failed (HTTP %d, %s) — check connectivity, environment, and BW_API_URL; retry when the token endpoint is available", e.StatusCode, e.Code)
}

func loginCommand(profile string) string {
	if profile == "" {
		profile = "default"
	}
	return "band auth login --profile " + shellescape.Quote(profile)
}
