package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".config", "band", "config.json")
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error: %v", err)
	}
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestLoadDefaults(t *testing.T) {
	// Point at a path that doesn't exist — should return defaults
	cfg, err := Load("/tmp/band-cli-test-nonexistent/config.json")
	if err != nil {
		t.Fatalf("Load() on missing file returned error: %v", err)
	}
	if cfg.Format != "json" {
		t.Errorf("default Format = %q, want %q", cfg.Format, "json")
	}
	if cfg.AccountID != "" || cfg.ClientID != "" {
		t.Errorf("expected empty defaults, got %+v", cfg)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	want := &Config{
		ClientID:    "my-client-id",
		AccountID:   "ACC123",
		Format:      "table",
		Environment: "test",
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("file permissions = %o, want 0600", perm)
		}
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if got.ClientID != want.ClientID {
		t.Errorf("ClientID = %q, want %q", got.ClientID, want.ClientID)
	}
	if got.AccountID != want.AccountID {
		t.Errorf("AccountID = %q, want %q", got.AccountID, want.AccountID)
	}
	if got.Format != want.Format {
		t.Errorf("Format = %q, want %q", got.Format, want.Format)
	}
	if got.Environment != want.Environment {
		t.Errorf("Environment = %q, want %q", got.Environment, want.Environment)
	}
}

func TestSaveCreatesNestedDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "config.json")

	cfg := &Config{Format: "json"}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save() with nested dirs error: %v", err)
	}

	// Verify parent dir permissions
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("Stat() on parent dir error: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0700 {
			t.Errorf("dir permissions = %o, want 0700", perm)
		}
	}
}

func TestEnvVarOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	base := &Config{
		ClientID:  "FROM_FILE",
		AccountID: "ACC_FROM_FILE",
		Format:    "json",
	}
	if err := Save(path, base); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	t.Setenv("BW_ACCOUNT_ID", "FROM_ENV")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Env overlay is applied at read time via ActiveProfileConfig,
	// not mutated into stored fields during Load.
	p := cfg.ActiveProfileConfig()
	if p.AccountID != "FROM_ENV" {
		t.Errorf("ActiveProfileConfig().AccountID = %q, want %q (env override)", p.AccountID, "FROM_ENV")
	}
	if p.ClientID != "FROM_FILE" {
		t.Errorf("ActiveProfileConfig().ClientID = %q, want %q", p.ClientID, "FROM_FILE")
	}
	// Stored fields must remain untouched so Save can't leak env values to disk.
	if cfg.AccountID != "ACC_FROM_FILE" {
		t.Errorf("stored cfg.AccountID = %q, want %q (Load must not mutate stored fields)", cfg.AccountID, "ACC_FROM_FILE")
	}
}

func TestActiveProfileConfig_Legacy(t *testing.T) {
	cfg := &Config{
		ClientID:  "legacy-id",
		AccountID: "legacy-acct",
	}
	p := cfg.ActiveProfileConfig()
	if p.ClientID != "legacy-id" {
		t.Errorf("ClientID = %q, want %q", p.ClientID, "legacy-id")
	}
	if p.AccountID != "legacy-acct" {
		t.Errorf("AccountID = %q, want %q", p.AccountID, "legacy-acct")
	}
}

func TestActiveProfileConfig_WithProfiles(t *testing.T) {
	cfg := &Config{
		ActiveProfile: "admin",
		Profiles: map[string]*Profile{
			"default": {ClientID: "default-id", AccountID: "default-acct"},
			"admin":   {ClientID: "admin-id", AccountID: ""},
		},
	}
	p := cfg.ActiveProfileConfig()
	if p.ClientID != "admin-id" {
		t.Errorf("ClientID = %q, want %q", p.ClientID, "admin-id")
	}
	if p.AccountID != "" {
		t.Errorf("AccountID = %q, want empty", p.AccountID)
	}
}

func TestSetProfile(t *testing.T) {
	cfg := &Config{Format: "json"}
	p := &Profile{ClientID: "new-id", AccountID: "new-acct", Accounts: []string{"a1", "a2"}}
	cfg.SetProfile("test", p)

	if cfg.ActiveProfile != "test" {
		t.Errorf("ActiveProfile = %q, want %q", cfg.ActiveProfile, "test")
	}
	if cfg.Profiles["test"].ClientID != "new-id" {
		t.Errorf("profile ClientID = %q, want %q", cfg.Profiles["test"].ClientID, "new-id")
	}
	// Legacy fields should be synced
	if cfg.ClientID != "new-id" {
		t.Errorf("legacy ClientID = %q, want %q", cfg.ClientID, "new-id")
	}
}

func TestSetProfile_MultipleProfiles(t *testing.T) {
	cfg := &Config{Format: "json"}
	cfg.SetProfile("first", &Profile{ClientID: "first-id", AccountID: "first-acct"})
	cfg.SetProfile("second", &Profile{ClientID: "second-id", AccountID: "second-acct"})

	// Second should be active
	if cfg.ActiveProfile != "second" {
		t.Errorf("ActiveProfile = %q, want %q", cfg.ActiveProfile, "second")
	}
	// First should still exist
	if cfg.Profiles["first"].ClientID != "first-id" {
		t.Errorf("first profile ClientID = %q, want %q", cfg.Profiles["first"].ClientID, "first-id")
	}
	// Both should be in profiles
	if len(cfg.Profiles) != 2 {
		t.Errorf("got %d profiles, want 2", len(cfg.Profiles))
	}
}

func TestProfileSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{Format: "json"}
	cfg.SetProfile("default", &Profile{ClientID: "def-id", AccountID: "def-acct", Accounts: []string{"a1"}})
	cfg.SetProfile("admin", &Profile{ClientID: "adm-id", Accounts: []string{}})
	cfg.ActiveProfile = "default" // switch back to default

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.ActiveProfile != "default" {
		t.Errorf("ActiveProfile = %q, want %q", loaded.ActiveProfile, "default")
	}
	if len(loaded.Profiles) != 2 {
		t.Fatalf("got %d profiles, want 2", len(loaded.Profiles))
	}
	if loaded.Profiles["admin"].ClientID != "adm-id" {
		t.Errorf("admin ClientID = %q, want %q", loaded.Profiles["admin"].ClientID, "adm-id")
	}
	if loaded.Profiles["default"].AccountID != "def-acct" {
		t.Errorf("default AccountID = %q, want %q", loaded.Profiles["default"].AccountID, "def-acct")
	}
}

func TestProfileRolesAndBuildRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{Format: "json"}
	cfg.SetProfile("default", &Profile{
		ClientID: "build-id",
		Roles:    []string{"HttpVoice", "HTTP Application Management"},
		Build:    true,
	})

	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	p := loaded.Profiles["default"]
	if !p.Build {
		t.Errorf("Build = false, want true")
	}
	if len(p.Roles) != 2 || p.Roles[0] != "HttpVoice" {
		t.Errorf("Roles = %v, want [HttpVoice, HTTP Application Management]", p.Roles)
	}
}

func TestHasMultipleEnvironments(t *testing.T) {
	tests := []struct {
		name     string
		profiles map[string]*Profile
		want     bool
	}{
		{
			name:     "no profiles",
			profiles: nil,
			want:     false,
		},
		{
			name: "single prod profile",
			profiles: map[string]*Profile{
				"default": {ClientID: "id1", Environment: ""},
			},
			want: false,
		},
		{
			name: "single custom env profile",
			profiles: map[string]*Profile{
				"default": {ClientID: "id1", Environment: "custom"},
			},
			want: false,
		},
		{
			name: "two profiles same env",
			profiles: map[string]*Profile{
				"a": {ClientID: "id1", Environment: "prod"},
				"b": {ClientID: "id2", Environment: ""},
			},
			want: false,
		},
		{
			name: "prod and custom",
			profiles: map[string]*Profile{
				"default":   {ClientID: "id1", Environment: ""},
				"secondary": {ClientID: "id2", Environment: "custom"},
			},
			want: true,
		},
		{
			name: "test and custom",
			profiles: map[string]*Profile{
				"test":   {ClientID: "id1", Environment: "test"},
				"custom": {ClientID: "id2", Environment: "custom"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Profiles: tt.profiles}
			if got := cfg.HasMultipleEnvironments(); got != tt.want {
				t.Errorf("HasMultipleEnvironments() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllEnvVarOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	base := &Config{Format: "json"}
	base.SetProfile("default", &Profile{ClientID: "fileclientid", AccountID: "fileaccount", Environment: "prod"})
	if err := Save(path, base); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	t.Setenv("BW_CLIENT_ID", "envclientid")
	t.Setenv("BW_ACCOUNT_ID", "envaccount")
	t.Setenv("BW_ENVIRONMENT", "custom")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	p := cfg.ActiveProfileConfig()
	if p.ClientID != "envclientid" {
		t.Errorf("ActiveProfileConfig().ClientID = %q, want %q", p.ClientID, "envclientid")
	}
	if p.AccountID != "envaccount" {
		t.Errorf("ActiveProfileConfig().AccountID = %q, want %q", p.AccountID, "envaccount")
	}
	if p.Environment != "custom" {
		t.Errorf("ActiveProfileConfig().Environment = %q, want %q", p.Environment, "custom")
	}

	// Stored profile must remain untouched.
	stored := cfg.Profiles["default"]
	if stored.ClientID != "fileclientid" || stored.AccountID != "fileaccount" || stored.Environment != "prod" {
		t.Errorf("stored profile mutated by Load: %+v", stored)
	}
}

// TestLoad_EnvOverlayDoesNotPersistOntoStoredProfiles guards against the
// regression where Load applied env vars to the live *Profile pointer in
// cfg.Profiles, so that any subsequent Save (login, switch, etc.) would
// silently rewrite the previously-active profile on disk with env values.
func TestLoad_EnvOverlayDoesNotPersistOntoStoredProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := &Config{Format: "json"}
	cfg.SetProfile("prod", &Profile{ClientID: "prod-id", AccountID: "ACCT_A", Environment: "prod"})
	cfg.SetProfile("dev", &Profile{ClientID: "dev-id", AccountID: "ACCT_B", Environment: "test"})
	cfg.ActiveProfile = "prod"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BW_ACCOUNT_ID", "ENV_ACCT_Z")
	t.Setenv("BW_CLIENT_ID", "ENV_CLIENT_Z")
	t.Setenv("BW_ENVIRONMENT", "ENV_HOST_Z")

	// Simulate a writer flow: Load → mutate something unrelated → Save.
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded.ActiveProfile = "dev"
	if err := Save(path, loaded); err != nil {
		t.Fatal(err)
	}

	// Re-read with env vars cleared to see only what was persisted.
	t.Setenv("BW_ACCOUNT_ID", "")
	t.Setenv("BW_CLIENT_ID", "")
	t.Setenv("BW_ENVIRONMENT", "")

	fresh, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	prod := fresh.Profiles["prod"]
	if prod.AccountID != "ACCT_A" || prod.ClientID != "prod-id" || prod.Environment != "prod" {
		t.Errorf("prod profile leaked env values: %+v", prod)
	}
	dev := fresh.Profiles["dev"]
	if dev.AccountID != "ACCT_B" || dev.ClientID != "dev-id" || dev.Environment != "test" {
		t.Errorf("dev profile leaked env values: %+v", dev)
	}
}

func TestActiveProfileConfig_AppliesEnvOverlay(t *testing.T) {
	cfg := &Config{}
	cfg.SetProfile("default", &Profile{ClientID: "id1", AccountID: "ACCT_A", Environment: "prod"})

	t.Setenv("BW_ACCOUNT_ID", "ENV_ACCT_Z")
	t.Setenv("BW_CLIENT_ID", "ENV_CLIENT_Z")
	t.Setenv("BW_ENVIRONMENT", "test")

	p := cfg.ActiveProfileConfig()
	if p.AccountID != "ENV_ACCT_Z" {
		t.Errorf("AccountID = %q, want %q", p.AccountID, "ENV_ACCT_Z")
	}
	if p.ClientID != "ENV_CLIENT_Z" {
		t.Errorf("ClientID = %q, want %q", p.ClientID, "ENV_CLIENT_Z")
	}
	if p.Environment != "test" {
		t.Errorf("Environment = %q, want %q", p.Environment, "test")
	}

	// Stored profile must not be mutated by ActiveProfileConfig.
	stored := cfg.Profiles["default"]
	if stored.AccountID != "ACCT_A" || stored.ClientID != "id1" || stored.Environment != "prod" {
		t.Errorf("stored profile mutated by ActiveProfileConfig: %+v", stored)
	}
}

func TestActiveProfileConfig_ReturnsCopySafeToMutate(t *testing.T) {
	cfg := &Config{}
	cfg.SetProfile("default", &Profile{ClientID: "id1", AccountID: "ACCT_A"})

	p := cfg.ActiveProfileConfig()
	p.AccountID = "MUTATED"

	if cfg.Profiles["default"].AccountID != "ACCT_A" {
		t.Errorf("mutating ActiveProfileConfig() result leaked into stored profile: %q", cfg.Profiles["default"].AccountID)
	}
}
