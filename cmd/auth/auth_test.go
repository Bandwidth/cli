package auth

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/config"
	"github.com/Bandwidth/cli/internal/testutil"
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
			name:  "messaging and voice",
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

func TestSIPCapability(t *testing.T) {
	// capabilities stays map[string]bool — a tri-state cannot live inside it,
	// so SIP is reported as a separate typed object.
	got := sipCapability(true)
	if got["status"] != "unknown" || got["reason"] != "role_present_not_probed" {
		t.Errorf("sipCapability(true) = %v, want unknown/role_present_not_probed", got)
	}
	got = sipCapability(false)
	if got["status"] != "unavailable" || got["reason"] != "role_absent" {
		t.Errorf("sipCapability(false) = %v, want unavailable/role_absent", got)
	}
}

// TestHasRole guards the matcher that decides whether sipCapability reports
// role_absent vs. role_present_not_probed. It mirrors TestCapabilities'
// fixture style, using the exact "SIP Credentials" string as it appears in
// the JWT roles array — a near-miss in casing or spacing here would silently
// report role_absent for an account that genuinely holds the role.
func TestHasRole(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		want  bool
	}{
		{
			name:  "exact role string as it appears in the JWT",
			roles: []string{"SIP Credentials"},
			want:  true,
		},
		{
			name:  "realistic full role slice with SIP present",
			roles: []string{"HTTP Application Management", "HttpVoice", "SIP Credentials", "brtcAccessRole"},
			want:  true,
		},
		{
			name:  "realistic full role slice without SIP",
			roles: []string{"HTTP Application Management", "HttpVoice", "brtcAccessRole"},
			want:  false,
		},
		{
			name:  "empty roles",
			roles: nil,
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasRole(tt.roles, "sip credentials"); got != tt.want {
				t.Errorf("hasRole(%v, %q) = %v, want %v", tt.roles, "sip credentials", got, tt.want)
			}
		})
	}
}

func TestTenDLCCapability(t *testing.T) {
	got := tendlcCapability(true)
	if got["status"] != "unknown" || got["reason"] != "role_present_not_probed" {
		t.Errorf("tendlcCapability(true) = %v, want unknown/role_present_not_probed", got)
	}
	got = tendlcCapability(false)
	if got["status"] != "unavailable" || got["reason"] != "role_absent" {
		t.Errorf("tendlcCapability(false) = %v, want unavailable/role_absent", got)
	}
}

// Customer Profiles Access is a distinct role from campaign_management —
// confirmed on a live credential that carries both.
func TestCustomerProfilesCapabilityIsSeparate(t *testing.T) {
	caps := Capabilities([]string{"campaign_management"})
	if caps["customer_profiles"] {
		t.Error("customer_profiles should be false without the Customer Profiles Access role")
	}
	if !caps["campaign_management"] {
		t.Error("campaign_management should be true")
	}

	caps = Capabilities([]string{"Customer Profiles Access"})
	if !caps["customer_profiles"] {
		t.Error("customer_profiles should be true with the role")
	}
	if caps["campaign_management"] {
		t.Error("campaign_management should be false without its own role")
	}
}

// liveAccount9901287Roles is the (trimmed) role list observed on a live
// credential for account 9901287 — captured directly rather than invented, so
// tests exercise the tri-state wiring against a realistic role string, not
// just an idealized one. In particular it confirms the role is genuinely
// snake_case "campaign_management" (not the "Campaign Management" display
// string used elsewhere for error messages), and it includes a decoy —
// "specialized customer external tns" — that contains the word "customer" but
// is not a Customer Profiles Access role.
var liveAccount9901287Roles = []string{
	"Alerting Insights", "Analytics", "Billing Reports",
	"campaign_management", "Configuration", "Customer Profiles Access",
	"Disconnect", "E911 Management", "specialized customer external tns",
	"HTTP Application Management", "HttpVoice", "Line Features",
	"Ordering", "Porting", "Reporting", "Short Code Access",
}

// removeRole returns a copy of roles with every occurrence of target removed.
func removeRole(roles []string, target string) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		if r != target {
			out = append(out, r)
		}
	}
	return out
}

// TestTenDLCWiringAgainstLiveRoles exercises hasRole, tendlcCapability, and
// Capabilities together against the real role strings from
// liveAccount9901287Roles, rather than testing tendlcCapability(bool) in
// isolation. TestTenDLCCapability and TestCustomerProfilesCapabilityIsSeparate
// already cover the functions' own logic; this test guards the wiring itself
// — a future change to hasRole's matching (e.g. adding normalization that
// treats "_" and " " differently, or an accidental typo in the substring
// passed at the call site) could silently make campaign_management always
// evaluate to role_absent while every existing test still passes.
func TestTenDLCWiringAgainstLiveRoles(t *testing.T) {
	if !hasRole(liveAccount9901287Roles, "campaign_management") {
		t.Fatal("hasRole should find campaign_management in the live role list")
	}
	got := tendlcCapability(hasRole(liveAccount9901287Roles, "campaign_management"))
	if got["status"] != "unknown" || got["reason"] != "role_present_not_probed" {
		t.Errorf("tendlcCapability with live roles = %v, want unknown/role_present_not_probed", got)
	}
	if !Capabilities(liveAccount9901287Roles)["customer_profiles"] {
		t.Error("customer_profiles should be true with live roles (Customer Profiles Access is present)")
	}

	withoutCampaign := removeRole(liveAccount9901287Roles, "campaign_management")
	if hasRole(withoutCampaign, "campaign_management") {
		t.Fatal("hasRole should not find campaign_management once it's removed from the role list")
	}
	got = tendlcCapability(hasRole(withoutCampaign, "campaign_management"))
	if got["status"] != "unavailable" || got["reason"] != "role_absent" {
		t.Errorf("tendlcCapability without campaign_management = %v, want unavailable/role_absent", got)
	}

	withoutCustomerProfiles := removeRole(liveAccount9901287Roles, "Customer Profiles Access")
	if Capabilities(withoutCustomerProfiles)["customer_profiles"] {
		t.Error("customer_profiles should be false once Customer Profiles Access is removed from the role list")
	}
}

// TestStatusPlainTenDLCAgreesWithCapabilities is an end-to-end regression
// test on `band auth status --plain`'s actual JSON output, not just on the
// helper functions in isolation. The bug lived in the call-site wiring
// inside runStatus (status.go), which fed tendlcCapability a *different*
// signal — hasRole(p.Roles, "campaign_management"), a plain substring match
// on the unnormalized snake_case string — than the one behind
// capabilities.campaign_management, which is Contains(rl, "campaign") over
// the same roles. For the display-form role "Campaign Management" those two
// signals disagree: the boolean matches (it contains "campaign") but the old
// hasRole lookup does not (no role contains the literal substring
// "campaign_management" with an underscore). That produced a single JSON
// document asserting both capabilities.campaign_management=true and
// tendlc={"status":"unavailable","reason":"role_absent"} for the same
// credential. Testing tendlcCapability(caps["campaign_management"]) directly
// would trivially pass by construction regardless of what runStatus itself
// does; only driving runStatus end to end (as done here) actually exercises
// the wiring and would have failed before the fix.
func TestStatusPlainTenDLCAgreesWithCapabilities(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
	}{
		{name: "snake_case role as seen on live account 9901287", roles: []string{"campaign_management"}},
		{name: "display-form role", roles: []string{"Campaign Management"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

			cfgPath, err := config.DefaultPath()
			if err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{Format: "json"}
			cfg.SetProfile("default", &config.Profile{
				ClientID:  "id1",
				AccountID: "ACCT_A",
				Roles:     tt.roles,
			})
			if err := config.Save(cfgPath, cfg); err != nil {
				t.Fatal(err)
			}

			wrap := &cobra.Command{Use: "status", RunE: runStatus}
			root := testutil.NewTestRoot(wrap)
			root.SetArgs([]string{"status", "--plain"})

			out := testutil.CaptureStdout(t, func() {
				if err := root.Execute(); err != nil {
					t.Fatalf("Execute() error = %v", err)
				}
			})

			var got statusJSON
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("unmarshal output: %v\noutput: %s", err, out)
			}

			if !got.Capabilities["campaign_management"] {
				t.Fatalf("capabilities.campaign_management = false for roles %v, want true", tt.roles)
			}
			if got.TenDLC["reason"] == "role_absent" {
				t.Errorf("capabilities.campaign_management = true but tendlc = %v — the two must agree in one JSON document", got.TenDLC)
			}
		})
	}
}

// TestCustomerProfilesMatcherNotOverBroad guards against loosening the
// customer_profiles matcher from "customer profiles" to just "customer".
// "specialized customer external tns" is a real role from
// liveAccount9901287Roles that contains "customer" but has nothing to do with
// Customer Profiles Access — it must not flip customer_profiles to true.
func TestCustomerProfilesMatcherNotOverBroad(t *testing.T) {
	caps := Capabilities([]string{"specialized customer external tns"})
	if caps["customer_profiles"] {
		t.Error("customer_profiles should be false for 'specialized customer external tns' — it contains 'customer' but is not a Customer Profiles Access role")
	}
}

// TestRunSwitch_PersistsTargetIntoActiveProfile guards against the bug where
// switch only updated the legacy top-level cfg.AccountID, leaving the active
// profile's AccountID stale — so subsequent commands continued targeting the
// pre-switch account.
func TestRunSwitch_PersistsTargetIntoActiveProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// On macOS, UserHomeDir checks HOME first, but ensure XDG_CONFIG_HOME isn't
	// pointing somewhere else for this test.
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	cfgPath, err := config.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Format: "json"}
	cfg.SetProfile("default", &config.Profile{
		ClientID:  "id1",
		AccountID: "ACCT_A",
		Accounts:  []string{"ACCT_A", "ACCT_B"},
	})
	if err := config.Save(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	if err := runSwitch(nil, []string{"ACCT_B"}); err != nil {
		t.Fatalf("runSwitch returned error: %v", err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	p := loaded.Profiles["default"]
	if p == nil {
		t.Fatal("default profile missing after switch")
	}
	if p.AccountID != "ACCT_B" {
		t.Errorf("profile AccountID after switch = %q, want %q", p.AccountID, "ACCT_B")
	}
	// Active-profile lookup must agree (this is what subsequent commands consult).
	active := loaded.ActiveProfileConfig()
	if active.AccountID != "ACCT_B" {
		t.Errorf("ActiveProfileConfig().AccountID after switch = %q, want %q", active.AccountID, "ACCT_B")
	}
}
