// Package cmdutil provides shared helpers for CLI command implementations.
package cmdutil

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/auth"
	"github.com/Bandwidth/cli/internal/config"
)

// apiHostForEnvironment maps an environment name to its API host.
// Non-production environments can be overridden with BW_API_URL.
func apiHostForEnvironment(env string) string {
	if v := os.Getenv("BW_API_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	switch env {
	case "test", "uat":
		return "https://test.api.bandwidth.com"
	default: // prod or empty
		return "https://api.bandwidth.com"
	}
}

// voiceHostForEnvironment maps an environment name to its Voice API host.
// Non-production environments can be overridden with BW_VOICE_URL.
func voiceHostForEnvironment(env string) string {
	if v := os.Getenv("BW_VOICE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	switch env {
	case "test", "uat":
		return "https://test.voice.bandwidth.com"
	default:
		return "https://voice.bandwidth.com"
	}
}

// loadConfigAndAuth loads the config, retrieves the client secret, and returns
// everything needed to build an API client.
func loadConfigAndAuth() (*config.Config, *config.Profile, string, error) {
	configPath, err := config.DefaultPath()
	if err != nil {
		return nil, nil, "", fmt.Errorf("resolving config path: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("loading config: %w", err)
	}

	p := cfg.ActiveProfileConfig()
	if p.ClientID == "" {
		return nil, nil, "", fmt.Errorf("not logged in — run `band auth login` first")
	}

	clientSecret, err := auth.GetPassword(p.ClientID)
	if err != nil {
		return nil, nil, "", fmt.Errorf("credentials not found in keychain for %s — run `band auth login`", p.ClientID)
	}

	return cfg, p, clientSecret, nil
}

// resolveAccountID resolves the account ID from override > env > config,
// returning an actionable error if none is set.
func resolveAccountID(cfg *config.Config, p *config.Profile, accountIDOverride string) (string, error) {
	acctID := accountIDOverride
	if acctID == "" {
		acctID = os.Getenv("BW_ACCOUNT_ID")
	}
	if acctID == "" {
		acctID = p.AccountID
	}
	if acctID != "" {
		return acctID, nil
	}

	// No account ID found — build a helpful error
	profileName := cfg.ActiveProfile
	if profileName == "" {
		profileName = "default"
	}

	if len(p.Accounts) > 0 {
		return "", fmt.Errorf("no active account set for profile %q.\n"+
			"Available accounts: %s\n"+
			"Run: band auth switch <account-id>\n"+
			"Or pass --account-id <id> on this command",
			profileName, strings.Join(p.Accounts, ", "))
	}

	return "", fmt.Errorf("no account ID set for profile %q.\n"+
		"This credential has system-wide access — pass --account-id <id> on this command.\n"+
		"Hint: use the default profile's accounts: band auth use default && band auth status",
		profileName)
}

// authenticate loads config, resolves the account, and returns a token manager
// plus the resolved environment and account ID.
func authenticate(accountIDOverride string) (*auth.TokenManager, string, string, error) {
	cfg, p, clientSecret, err := loadConfigAndAuth()
	if err != nil {
		return nil, "", "", err
	}

	acctID, err := resolveAccountID(cfg, p, accountIDOverride)
	if err != nil {
		return nil, "", "", err
	}

	apiHost := apiHostForEnvironment(p.Environment)
	tm := auth.NewTokenManager(p.ClientID, clientSecret, apiHost)
	return tm, acctID, p.Environment, nil
}

// BuildClient returns an authenticated JSON API client.
func BuildClient(apiBaseURL, accountIDOverride string) (*api.Client, string, error) {
	tm, acctID, _, err := authenticate(accountIDOverride)
	if err != nil {
		return nil, "", err
	}
	return api.NewClient(apiBaseURL, tm), acctID, nil
}

// BuildXMLClient returns an authenticated XML-mode client for the Dashboard API.
func BuildXMLClient(apiBaseURL, accountIDOverride string) (*api.Client, string, error) {
	tm, acctID, _, err := authenticate(accountIDOverride)
	if err != nil {
		return nil, "", err
	}
	return api.NewXMLClient(apiBaseURL, tm), acctID, nil
}

// OutputFlags extracts the common --plain and --format flags from a command's root.
func OutputFlags(cmd *cobra.Command) (format string, plain bool) {
	plain = cmd.Root().Flag("plain").Value.String() == "true"
	format = cmd.Root().Flag("format").Value.String()
	return format, plain
}

// AccountIDFlag extracts the --account-id override from a command's root.
func AccountIDFlag(cmd *cobra.Command) string {
	return cmd.Root().Flag("account-id").Value.String()
}

// DashboardClient returns an XML-mode client for the Bandwidth Dashboard API v2.
func DashboardClient(accountIDOverride string) (*api.Client, string, error) {
	tm, acctID, env, err := authenticate(accountIDOverride)
	if err != nil {
		return nil, "", err
	}
	return api.NewXMLClient(apiHostForEnvironment(env)+"/api/v2", tm), acctID, nil
}

// VoiceClient returns a client for the Bandwidth Voice API v2.
func VoiceClient(accountIDOverride string) (*api.Client, string, error) {
	tm, acctID, env, err := authenticate(accountIDOverride)
	if err != nil {
		return nil, "", err
	}
	return api.NewClient(voiceHostForEnvironment(env)+"/api/v2", tm), acctID, nil
}

// PlatformClient creates a JSON API client for Universal Platform v2 endpoints (e.g. VCP).
func PlatformClient(accountIDOverride string) (*api.Client, string, error) {
	tm, acctID, env, err := authenticate(accountIDOverride)
	if err != nil {
		return nil, "", err
	}
	return api.NewClient(apiHostForEnvironment(env), tm), acctID, nil
}

// MessagingClient returns a client for the Bandwidth Messaging API v2.
func MessagingClient(accountIDOverride string) (*api.Client, string, error) {
	return BuildClient("https://messaging.bandwidth.com/api/v2", accountIDOverride)
}

