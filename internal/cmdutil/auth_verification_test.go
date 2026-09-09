package cmdutil

import (
	"testing"

	"github.com/Bandwidth/cli/internal/auth"
	"github.com/Bandwidth/cli/internal/config"
)

func TestAuthTokenManagerEnvironment(t *testing.T) {
	for _, tc := range []struct {
		profile, override, url, wantHost, wantEnv string
		wantError                                 bool
	}{
		{"", "", "", "https://api.bandwidth.com", "prod", false},
		{"test", "", "", "https://test.api.bandwidth.com", "test", false},
		{"prod", "test", "", "https://test.api.bandwidth.com", "test", false},
		{"test", "prod", "", "https://api.bandwidth.com", "prod", false},
		{"stage", "", "https://custom.example/", "https://custom.example", "stage", false},
		{"typo", "", "", "", "", true},
		{"prod", "", "missing-scheme", "", "", true},
	} {
		t.Run(tc.profile+tc.override+tc.url, func(t *testing.T) {
			t.Setenv("BW_API_URL", tc.url)
			old := EnvironmentOverride
			EnvironmentOverride = tc.override
			t.Cleanup(func() { EnvironmentOverride = old })
			tm, env, err := AuthTokenManager(&config.Profile{ClientID: "id", Environment: tc.profile}, "secret", "admin")
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v", err)
			}
			if err != nil {
				return
			}
			if env != tc.wantEnv || tm.TokenURL != tc.wantHost || tm.ProfileName != "admin" {
				t.Fatalf("wrong environment or profile: %s %s %s", env, tm.TokenURL, tm.ProfileName)
			}
		})
	}
}

func TestMissingCredentialsExitAuth(t *testing.T) {
	err := &auth.CredentialError{Reason: "not_logged_in", Profile: "admin"}
	if got := ExitCodeForError(err); got != ExitAuth {
		t.Fatalf("exit = %d", got)
	}
}
