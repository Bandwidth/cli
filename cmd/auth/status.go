package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	intauth "github.com/Bandwidth/cli/internal/auth"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/config"
	"github.com/Bandwidth/cli/internal/ui"
)

func init() {
	Cmd.AddCommand(statusCmd)
	statusCmd.Flags().Bool("no-verify", false, "Inspect stored credentials without contacting the token endpoint (authenticated remains false)")
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	Long:  "Verifies the active profile's credentials with a fresh token exchange. Use --no-verify for offline inspection, or --plain for machine-readable JSON. SIP and 10DLC account-level availability still require their separate status probes.",
	Example: `  band auth status
  band auth status --plain`,
	RunE: runStatus,
	Args: cobra.NoArgs,
}

// statusJSON is the structured output shape returned when --plain is set.
// Existing fields are retained; authenticated now requires verification.
// Offline consumers migrate to credentials_stored (see AGENTS.md).
type statusJSON struct {
	Authenticated     bool            `json:"authenticated"`
	CredentialsStored bool            `json:"credentials_stored"`
	Token             tokenStatus     `json:"token"`
	Profile           string          `json:"profile,omitempty"`
	ClientID          string          `json:"client_id,omitempty"`
	AccountID         string          `json:"account_id,omitempty"`
	Accounts          []string        `json:"accounts,omitempty"`
	Environment       string          `json:"environment,omitempty"`
	Build             bool            `json:"build,omitempty"`
	Roles             []string        `json:"roles,omitempty"`
	Capabilities      map[string]bool `json:"capabilities,omitempty"`
	// SIP reports SIP provisioning availability as a tri-state object
	// ({"status":..., "reason":...}) rather than a bool inside Capabilities —
	// see sipCapability.
	SIP map[string]string `json:"sip,omitempty"`
	// TenDLC reports Registration Center availability as a tri-state, for the
	// same reason as SIP — see tendlcCapability.
	TenDLC map[string]string `json:"tendlc,omitempty"`
	Error  string            `json:"error,omitempty"`
}

type tokenStatus struct {
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	ExpiresIn *int   `json:"expires_in,omitempty"`
}

// Seamed for tests so no real OS keychain is read.
var statusPassword = intauth.GetPassword

func runStatus(cmd *cobra.Command, args []string) error {
	_, plain := cmdutil.OutputFlags(cmd)

	configPath, err := config.DefaultPath()
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p := cfg.ActiveProfileConfig()

	env := p.Environment
	if env == "" {
		env = "prod"
	}

	profileName := cfg.ActiveProfile
	if profileName == "" {
		profileName = "default"
	}

	noVerify, _ := cmd.Flags().GetBool("no-verify")
	caps := Capabilities(p.Roles)
	out := statusJSON{
		Profile:      profileName,
		ClientID:     p.ClientID,
		AccountID:    p.AccountID,
		Accounts:     p.Accounts,
		Environment:  env,
		Build:        p.Build,
		Roles:        p.Roles,
		Capabilities: caps,
		SIP:          sipCapability(hasRole(p.Roles, "sip credentials")),
		TenDLC:       tendlcCapability(caps["campaign_management"]),
	}
	out.Token = tokenStatus{Status: "unknown", Reason: "not_verified"}
	var verifyErr error
	var secret string
	if p.ClientID == "" {
		out.Token.Reason = "not_logged_in"
		verifyErr = &intauth.CredentialError{Reason: out.Token.Reason, Profile: profileName}
	} else {
		secret, err = statusPassword(p.ClientID)
		out.CredentialsStored = err == nil && secret != ""
		if !out.CredentialsStored {
			out.Token.Reason = "credentials_unavailable"
			verifyErr = &intauth.CredentialError{Reason: out.Token.Reason, Profile: profileName}
		}
	}
	if !noVerify && verifyErr == nil {
		var tm *intauth.TokenManager
		tm, out.Environment, verifyErr = cmdutil.AuthTokenManager(p, secret, profileName)
		if verifyErr == nil {
			var token string
			var expires int
			token, expires, verifyErr = tm.Verify(cmd.Context())
			if verifyErr == nil {
				var claims *jwtClaims
				claims, verifyErr = parseJWTClaims(token)
				if verifyErr == nil {
					out.Authenticated = true
					out.Token = tokenStatus{Status: "valid", ExpiresIn: &expires}
					out.Accounts, out.Roles, out.Build = claims.Accounts, claims.Roles, claims.Build
					out.Capabilities = Capabilities(claims.Roles)
					out.SIP = sipCapability(hasRole(claims.Roles, "sip credentials"))
					out.TenDLC = tendlcCapability(out.Capabilities["campaign_management"])
				}
			}
		}
		if verifyErr != nil {
			out.Token = tokenStatus{Status: "unknown", Reason: "probe_failed"}
			var tokenErr *intauth.TokenError
			if errors.As(verifyErr, &tokenErr) && tokenErr.Rejected() {
				out.Token = tokenStatus{Status: "rejected", Reason: tokenErr.Code}
			}
		}
	}
	if verifyErr != nil {
		out.Error = verifyErr.Error()
	}
	if plain {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return err
		}
	} else {
		w := cmd.ErrOrStderr()
		fmt.Fprintf(w, "Profile:     %s\nClient ID:   %s\nAccount:     %s\nEnvironment: %s\n", out.Profile, out.ClientID, out.AccountID, out.Environment)
		fmt.Fprintf(w, "Status:      %s", out.Token.Status)
		if out.Token.Reason != "" {
			fmt.Fprintf(w, " (%s)", out.Token.Reason)
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Accounts:    %s\n", strings.Join(out.Accounts, ", "))
		if out.Build {
			fmt.Fprintln(w, "Type:        Bandwidth Build (voice-only, credit-based)")
		}
		fmt.Fprintf(w, "Capable of:  %s\nSIP:         %s\n10DLC:       %s\n", capabilitySummary(out.Capabilities), sipSummary(out.SIP), tendlcSummary(out.TenDLC))
		if len(cfg.Profiles) > 1 {
			fmt.Fprintf(w, "Profiles:    %s\n", strings.Join(cfg.ProfileNames(), ", "))
		}
	}
	if noVerify {
		return nil
	}
	return verifyErr
}

// Capabilities maps a set of JWT role strings to a stable feature map.
// Unknown roles are ignored; absence of a known role means the capability
// is false. Conservative by design — better to omit than to over-promise.
func Capabilities(roles []string) map[string]bool {
	caps := map[string]bool{
		"voice":               false,
		"app_management":      false,
		"messaging":           false,
		"numbers":             false,
		"vcp":                 false,
		"campaign_management": false,
		"customer_profiles":   false,
		"tfv":                 false,
	}
	for _, r := range roles {
		rl := strings.ToLower(r)
		if strings.Contains(rl, "httpvoice") || strings.Contains(rl, " voice") {
			caps["voice"] = true
		}
		if strings.Contains(rl, "application management") || strings.Contains(rl, "app management") {
			caps["app_management"] = true
		}
		if strings.Contains(rl, "messag") || strings.Contains(rl, "sms") {
			caps["messaging"] = true
		}
		if strings.Contains(rl, "number") {
			caps["numbers"] = true
		}
		if strings.Contains(rl, "vcp") || strings.Contains(rl, "voice configuration") {
			caps["vcp"] = true
		}
		if strings.Contains(rl, "campaign") {
			caps["campaign_management"] = true
		}
		if strings.Contains(rl, "customer profiles") {
			caps["customer_profiles"] = true
		}
		if strings.Contains(rl, "tfv") || strings.Contains(rl, "toll-free") || strings.Contains(rl, "tollfree") {
			caps["tfv"] = true
		}
	}
	return caps
}

// hasRole reports whether any role string in roles contains substr,
// case-insensitively — the same matching style Capabilities uses.
func hasRole(roles []string, substr string) bool {
	for _, r := range roles {
		if strings.Contains(strings.ToLower(r), substr) {
			return true
		}
	}
	return false
}

// sipCapability reports SIP provisioning availability as a tri-state. SIP needs
// both the "SIP Credentials" role and account-level SipCredentialSettings, and
// only the role is knowable offline — so a boolean would be misleading.
// Reasons are stable identifiers, not prose. The full set, across this offline
// derivation and the `band sip status` probe: role_absent,
// role_present_not_probed, probe_succeeded, account_not_enabled, probe_failed.
// This function only ever emits the first two — it stays offline.
func sipCapability(hasRole bool) map[string]string {
	if !hasRole {
		return map[string]string{"status": "unavailable", "reason": "role_absent"}
	}
	return map[string]string{"status": "unknown", "reason": "role_present_not_probed"}
}

// sipSummary renders the offline SIP tri-state for the human-readable auth
// status output. reason values are internal identifiers (see sipCapability);
// only this function turns them into prose.
func sipSummary(sip map[string]string) string {
	switch sip["reason"] {
	case "role_absent":
		return ui.Muted("not available (missing SIP Credentials role)")
	case "role_present_not_probed":
		return ui.Muted("unknown — run 'band sip status' to check")
	default:
		return sip["status"]
	}
}

// tendlcCapability reports 10DLC Registration Center availability as a
// tri-state. Access needs both the Campaign Management role and the
// account-level Registration Center feature; only the role is knowable
// offline, so a boolean would over-promise. Mirrors sipCapability.
//
// Callers must pass caps["campaign_management"] from the same Capabilities()
// call used for the campaign_management boolean, not a separately-matched
// hasRole lookup — otherwise the two can disagree on a single credential
// (e.g. a display-form role string) even though they describe the same fact.
func tendlcCapability(hasRole bool) map[string]string {
	if !hasRole {
		return map[string]string{"status": "unavailable", "reason": "role_absent"}
	}
	return map[string]string{"status": "unknown", "reason": "role_present_not_probed"}
}

// tendlcSummary renders the offline tri-state for human-readable output.
func tendlcSummary(t map[string]string) string {
	switch t["reason"] {
	case "role_absent":
		return ui.Muted("not available (missing Campaign Management role)")
	case "role_present_not_probed":
		return ui.Muted("unknown — run 'band tendlc status' to check")
	default:
		return t["status"]
	}
}

// capabilitySummary renders a capability map as a "have / not" line
// for the human-readable auth status output on Build accounts.
func capabilitySummary(caps map[string]bool) string {
	labels := map[string]string{
		"voice":               "voice",
		"app_management":      "app management",
		"messaging":           "messaging",
		"numbers":             "number ordering",
		"vcp":                 "VCP",
		"campaign_management": "10DLC campaigns",
		"tfv":                 "toll-free verification",
	}
	order := []string{"voice", "app_management", "messaging", "numbers", "vcp", "campaign_management", "tfv"}
	var have, missing []string
	for _, k := range order {
		if caps[k] {
			have = append(have, labels[k])
		} else {
			missing = append(missing, labels[k])
		}
	}
	out := strings.Join(have, ", ")
	if len(missing) > 0 {
		out += " " + ui.Muted("(no "+strings.Join(missing, ", ")+")")
	}
	return out
}
