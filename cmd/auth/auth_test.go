package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "auth" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "auth")
	}

	subs := map[string]bool{}
	for _, c := range Cmd.Commands() {
		subs[c.Use] = true
	}
	for _, name := range []string{"login", "logout", "status", "switch [account-id]", "profiles"} {
		if !subs[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestTokenURLForEnvironment(t *testing.T) {
	tests := []struct {
		env  string
		want string
	}{
		{"prod", "https://api.bandwidth.com"},
		{"", "https://api.bandwidth.com"},
		{"test", "https://test.api.bandwidth.com"},
		{"uat", "https://test.api.bandwidth.com"},
		{"unknown env", "https://api.bandwidth.com"},
	}
	for _, tc := range tests {
		t.Run(tc.env, func(t *testing.T) {
			got := tokenURLForEnvironment(tc.env)
			if got != tc.want {
				t.Errorf("tokenURLForEnvironment(%q) = %q, want %q", tc.env, got, tc.want)
			}
		})
	}
}

func TestParseJWTClaims(t *testing.T) {
	claims := map[string]any{
		"accounts":   []string{"9900001", "9900002"},
		"acct_scope": "9900001",
		"roles":      []string{"admin"},
	}
	payload, _ := json.Marshal(claims)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	token := "eyJhbGciOiJSUzI1NiJ9." + encoded + ".fakesig"

	parsed, err := parseJWTClaims(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.AcctScope != "9900001" {
		t.Errorf("AcctScope = %q, want %q", parsed.AcctScope, "9900001")
	}
	if len(parsed.Accounts) != 2 || parsed.Accounts[0] != "9900001" {
		t.Errorf("Accounts = %v, want [9900001 9900002]", parsed.Accounts)
	}
}

func TestParseJWTClaimsInvalidFormat(t *testing.T) {
	_, err := parseJWTClaims("not-a-jwt")
	if err == nil {
		t.Fatal("expected error for invalid JWT")
	}
}

func TestParseJWTClaimsInvalidPayload(t *testing.T) {
	_, err := parseJWTClaims("header.!!!invalid-base64!!!.sig")
	if err == nil {
		t.Fatal("expected error for invalid base64 payload")
	}
}
