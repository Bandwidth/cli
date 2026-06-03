package cmdutil

import "testing"

func TestVoiceHostForEnvironment(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{"prod default", "", "https://voice.bandwidth.com"},
		{"prod explicit", "prod", "https://voice.bandwidth.com"},
		{"unknown env falls back to prod", "other", "https://voice.bandwidth.com"},
		{"test", "test", "https://test.voice.bandwidth.com"},
		{"uat", "uat", "https://test.voice.bandwidth.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := voiceHostForEnvironment(tt.env); got != tt.want {
				t.Errorf("voiceHostForEnvironment(%q) = %q, want %q", tt.env, got, tt.want)
			}
		})
	}
}

func TestVoiceHostForEnvironment_BW_VOICE_URL(t *testing.T) {
	t.Setenv("BW_VOICE_URL", "https://custom.voice.example.com")
	for _, env := range []string{"", "prod", "test"} {
		got := voiceHostForEnvironment(env)
		if got != "https://custom.voice.example.com" {
			t.Errorf("voiceHostForEnvironment(%q) with BW_VOICE_URL = %q, want override", env, got)
		}
	}
}

func TestVoiceHostForEnvironment_BW_VOICE_URL_TrailingSlash(t *testing.T) {
	t.Setenv("BW_VOICE_URL", "https://custom.voice.example.com/")
	got := voiceHostForEnvironment("")
	if got != "https://custom.voice.example.com" {
		t.Errorf("voiceHostForEnvironment with trailing slash = %q, want without slash", got)
	}
}

func TestMessagingHost(t *testing.T) {
	// Messaging is production-only — there is no test/sandbox host, so the host
	// never varies by --environment. Only BW_MESSAGING_URL can override it.
	t.Run("prod default", func(t *testing.T) {
		if got := messagingHost(); got != "https://messaging.bandwidth.com" {
			t.Errorf("messagingHost() = %q, want https://messaging.bandwidth.com", got)
		}
	})
}

func TestMessagingHost_BW_MESSAGING_URL(t *testing.T) {
	t.Setenv("BW_MESSAGING_URL", "https://custom.messaging.example.com")
	if got := messagingHost(); got != "https://custom.messaging.example.com" {
		t.Errorf("messagingHost() with BW_MESSAGING_URL = %q, want override", got)
	}
}

func TestMessagingHost_BW_MESSAGING_URL_TrailingSlash(t *testing.T) {
	t.Setenv("BW_MESSAGING_URL", "http://localhost:8080/")
	if got := messagingHost(); got != "http://localhost:8080" {
		t.Errorf("messagingHost() with trailing slash = %q, want without slash", got)
	}
}

func TestResolveEnvironment(t *testing.T) {
	t.Run("no override returns profile env", func(t *testing.T) {
		EnvironmentOverride = ""
		got, err := resolveEnvironment("prod")
		if err != nil || got != "prod" {
			t.Errorf("got %q, err %v; want prod, nil", got, err)
		}
	})
	t.Run("override wins over profile env", func(t *testing.T) {
		EnvironmentOverride = "test"
		t.Cleanup(func() { EnvironmentOverride = "" })
		got, err := resolveEnvironment("prod")
		if err != nil || got != "test" {
			t.Errorf("got %q, err %v; want test, nil", got, err)
		}
	})
	t.Run("normalizes case and whitespace", func(t *testing.T) {
		EnvironmentOverride = "  TEST "
		t.Cleanup(func() { EnvironmentOverride = "" })
		got, err := resolveEnvironment("prod")
		if err != nil || got != "test" {
			t.Errorf("got %q, err %v; want test, nil", got, err)
		}
	})
	t.Run("unknown env is an error (no silent prod fall-through)", func(t *testing.T) {
		EnvironmentOverride = "staging"
		t.Cleanup(func() { EnvironmentOverride = "" })
		if _, err := resolveEnvironment("prod"); err == nil {
			t.Error("expected error for unknown env, got nil")
		}
	})
}

func TestMessagingProdOnlyWarning(t *testing.T) {
	for _, env := range []string{"test", "uat"} {
		if messagingProdOnlyWarning(env) == "" {
			t.Errorf("expected a warning for env %q", env)
		}
	}
	for _, env := range []string{"", "prod", "staging"} {
		if messagingProdOnlyWarning(env) != "" {
			t.Errorf("expected NO warning for env %q, got one", env)
		}
	}
}

func TestAPIHostForEnvironment(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{"prod default", "", "https://api.bandwidth.com"},
		{"prod explicit", "prod", "https://api.bandwidth.com"},
		{"unknown env falls back to prod", "other", "https://api.bandwidth.com"},
		{"test", "test", "https://test.api.bandwidth.com"},
		{"uat", "uat", "https://test.api.bandwidth.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apiHostForEnvironment(tt.env); got != tt.want {
				t.Errorf("apiHostForEnvironment(%q) = %q, want %q", tt.env, got, tt.want)
			}
		})
	}
}
