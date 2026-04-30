package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	intauth "github.com/Bandwidth/cli/internal/auth"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/config"
	"github.com/Bandwidth/cli/internal/ui"
)

func init() {
	Cmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE:  runStatus,
}

// statusJSON is the structured output shape returned when --plain is set.
// Stable contract for agents — additive changes only.
type statusJSON struct {
	Authenticated bool            `json:"authenticated"`
	Profile       string          `json:"profile,omitempty"`
	ClientID      string          `json:"client_id,omitempty"`
	AccountID     string          `json:"account_id,omitempty"`
	Accounts      []string        `json:"accounts,omitempty"`
	Environment   string          `json:"environment,omitempty"`
	Build         bool            `json:"build,omitempty"`
	Roles         []string        `json:"roles,omitempty"`
	Capabilities  map[string]bool `json:"capabilities,omitempty"`
	Error         string          `json:"error,omitempty"`
}

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

	if p.ClientID == "" {
		if plain {
			return emitJSON(statusJSON{Authenticated: false})
		}
		fmt.Fprintln(os.Stderr, ui.Warn("Not logged in."))
		return nil
	}

	env := p.Environment
	if env == "" {
		env = "prod"
	}

	profileName := cfg.ActiveProfile
	if profileName == "" {
		profileName = "default"
	}

	_, keychainErr := intauth.GetPassword(p.ClientID)

	if plain {
		out := statusJSON{
			Authenticated: keychainErr == nil,
			Profile:       profileName,
			ClientID:      p.ClientID,
			AccountID:     p.AccountID,
			Accounts:      p.Accounts,
			Environment:   env,
			Build:         p.Build,
			Roles:         p.Roles,
			Capabilities:  Capabilities(p.Roles),
		}
		if keychainErr != nil {
			out.Error = "credentials not found in keychain"
		}
		return emitJSON(out)
	}

	if keychainErr != nil {
		fmt.Printf("Client ID:   %s\n", ui.ID(p.ClientID))
		fmt.Printf("Account:     %s\n", ui.ID(p.AccountID))
		// Show environment only when it's informative.
		if env != "prod" || cfg.HasMultipleEnvironments() {
			fmt.Printf("Environment: %s\n", env)
		}
		fmt.Println("Status:      " + ui.Error("credentials not found in keychain"))
		return nil
	}

	fmt.Printf("Profile:     %s\n", ui.Bold(profileName))
	fmt.Printf("Client ID:   %s\n", ui.ID(p.ClientID))
	if p.AccountID != "" {
		fmt.Printf("Account:     %s\n", ui.ID(p.AccountID))
	} else {
		fmt.Printf("Account:     (none — pass --account-id on commands)\n")
	}
	if len(p.Accounts) > 1 {
		fmt.Printf("Accounts:    %s\n", strings.Join(p.Accounts, ", "))
	} else if len(p.Accounts) == 0 && p.AccountID == "" {
		fmt.Println("Scope:       system-wide (use --account-id to target an account)")
	}
	if p.Build {
		fmt.Printf("Type:        %s (voice-only, credit-based)\n", ui.Bold("Bandwidth Build"))
		fmt.Printf("Capable of:  %s\n", capabilitySummary(Capabilities(p.Roles)))
	}
	if env != "prod" || cfg.HasMultipleEnvironments() {
		fmt.Printf("Environment: %s\n", env)
	}
	fmt.Println("Status:      " + ui.Success("authenticated"))

	if len(cfg.Profiles) > 1 {
		fmt.Printf("Profiles:    %s\n", strings.Join(cfg.ProfileNames(), ", "))
	}
	return nil
}

func emitJSON(v statusJSON) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
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
		if strings.Contains(rl, "tfv") || strings.Contains(rl, "toll-free") || strings.Contains(rl, "tollfree") {
			caps["tfv"] = true
		}
	}
	return caps
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
