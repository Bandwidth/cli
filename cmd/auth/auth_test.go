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

func TestCapabilities(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		want  map[string]bool
	}{
		{
			name:  "build account roles",
			roles: []string{"HTTP Application Management", "HttpVoice", "brtcAccessRole"},
			want: map[string]bool{
				"voice":               true,
				"app_management":      true,
				"messaging":           false,
				"numbers":             false,
				"vcp":                 false,
				"campaign_management": false,
				"tfv":                 false,
			},
		},
		{
			name:  "no roles",
			roles: nil,
			want: map[string]bool{
				"voice":               false,
				"app_management":      false,
				"messaging":           false,
				"numbers":             false,
				"vcp":                 false,
				"campaign_management": false,
				"tfv":                 false,
			},
		},
		{
			name:  "messaging-capable",
			roles: []string{"Messaging", "HttpVoice"},
			want: map[string]bool{
				"voice":               true,
				"app_management":      false,
				"messaging":           true,
				"numbers":             false,
				"vcp":                 false,
				"campaign_management": false,
				"tfv":                 false,
			},
		},
		{
			name:  "campaign and tfv",
			roles: []string{"Campaign Management", "TFV"},
			want: map[string]bool{
				"voice":               false,
				"app_management":      false,
				"messaging":           false,
				"numbers":             false,
				"vcp":                 false,
				"campaign_management": true,
				"tfv":                 true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Capabilities(tt.roles)
			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("Capabilities[%q] = %v, want %v (roles=%v)", k, got[k], want, tt.roles)
				}
			}
		})
	}
}

func TestParseJWTClaims(t *testing.T) {
	claims := map[string]any{
		"accounts": []string{"9900001", "9900002"},
		"roles":    []string{"admin"},
		"express":  true,
	}
	payload, _ := json.Marshal(claims)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	token := "eyJhbGciOiJSUzI1NiJ9." + encoded + ".fakesig"

	parsed, err := parseJWTClaims(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parsed.Accounts) != 2 || parsed.Accounts[0] != "9900001" {
		t.Errorf("Accounts = %v, want [9900001 9900002]", parsed.Accounts)
	}
	if !parsed.Build {
		t.Errorf("Build = false, want true")
	}
	if len(parsed.Roles) != 1 || parsed.Roles[0] != "admin" {
		t.Errorf("Roles = %v, want [admin]", parsed.Roles)
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
