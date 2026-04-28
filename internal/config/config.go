package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Profile holds credentials and account info for a single set of API credentials.
type Profile struct {
	ClientID    string   `json:"client_id,omitempty"`
	AccountID   string   `json:"account_id,omitempty"`
	Accounts    []string `json:"accounts,omitempty"`
	Environment string   `json:"environment,omitempty"` // prod, test

	// Roles lists the JWT-granted role names on this credential. Used to
	// gate commands locally and produce capability hints in `auth status`.
	Roles []string `json:"roles,omitempty"`

	// Build is true when the credential is for a Bandwidth Build account
	// (voice-only, credit-based).
	Build bool `json:"build,omitempty"`
}

// Config holds the CLI configuration values persisted to ~/.band/config.json.
type Config struct {
	// Active profile name
	ActiveProfile string `json:"active_profile,omitempty"`

	// Named profiles (keyed by profile name)
	Profiles map[string]*Profile `json:"profiles,omitempty"`

	// Global settings
	Format string `json:"format,omitempty"`

	// Legacy fields — kept for backward compatibility during migration.
	// If present and Profiles is empty, auto-migrate to a "default" profile.
	ClientID    string   `json:"client_id,omitempty"`
	AccountID   string   `json:"account_id,omitempty"`
	Accounts    []string `json:"accounts,omitempty"`
	Environment string   `json:"environment,omitempty"`
}

// ActiveProfileConfig returns the active profile, resolving legacy config if needed.
func (c *Config) ActiveProfileConfig() *Profile {
	// If we have profiles, use the active one
	if len(c.Profiles) > 0 {
		name := c.ActiveProfile
		if name == "" {
			name = "default"
		}
		if p, ok := c.Profiles[name]; ok {
			return p
		}
		// Active profile doesn't exist — return first available
		for _, p := range c.Profiles {
			return p
		}
	}

	// Legacy: no profiles map, use top-level fields
	return &Profile{
		ClientID:    c.ClientID,
		AccountID:   c.AccountID,
		Accounts:    c.Accounts,
		Environment: c.Environment,
	}
}

// SetProfile stores a profile and sets it as active.
func (c *Config) SetProfile(name string, p *Profile) {
	if c.Profiles == nil {
		c.Profiles = make(map[string]*Profile)
	}
	c.Profiles[name] = p
	c.ActiveProfile = name

	// Also set legacy fields so older code that reads them directly still works
	c.ClientID = p.ClientID
	c.AccountID = p.AccountID
	c.Accounts = p.Accounts
	c.Environment = p.Environment
}

// ProfileNames returns sorted profile names.
func (c *Config) ProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for k := range c.Profiles {
		names = append(names, k)
	}
	return names
}

// HasMultipleEnvironments reports whether the configured profiles span more
// than one distinct environment. This is used to decide whether to surface the
// active environment in CLI output — a customer with only prod credentials
// doesn't need to see "Environment: production" on every command.
func (c *Config) HasMultipleEnvironments() bool {
	seen := make(map[string]bool)
	for _, p := range c.Profiles {
		env := p.Environment
		if env == "" {
			env = "prod"
		}
		seen[env] = true
		if len(seen) > 1 {
			return true
		}
	}
	return false
}

// DefaultPath returns the default config file path.
// Prefers the XDG-compliant ~/.config/band/config.json, but falls back to the
// legacy ~/.band/config.json if it exists and the new path doesn't (auto-migration).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	newPath := filepath.Join(home, ".config", "band", "config.json")
	legacyPath := filepath.Join(home, ".band", "config.json")

	// Use legacy path only if it exists and the new path does not yet exist.
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		if _, err := os.Stat(legacyPath); err == nil {
			return legacyPath, nil
		}
	}

	return newPath, nil
}

// Load reads config from path, overlays env vars, and returns defaults if the
// file does not exist. The default format is "json".
func Load(path string) (*Config, error) {
	cfg := &Config{Format: "json"}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// Overlay environment variables on the active profile
	p := cfg.ActiveProfileConfig()
	if v := os.Getenv("BW_CLIENT_ID"); v != "" {
		p.ClientID = v
		cfg.ClientID = v
	}
	if v := os.Getenv("BW_ACCOUNT_ID"); v != "" {
		p.AccountID = v
		cfg.AccountID = v
	}
	if v := os.Getenv("BW_FORMAT"); v != "" {
		cfg.Format = v
	}
	if v := os.Getenv("BW_ENVIRONMENT"); v != "" {
		p.Environment = v
		cfg.Environment = v
	}

	return cfg, nil
}

// Save writes cfg as JSON to path, creating parent directories as needed.
// Directories are created with 0700 permissions; the file is written with 0600.
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
