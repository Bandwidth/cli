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

func TestMessagingHostForEnvironment(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{"prod default", "", "https://messaging.bandwidth.com"},
		{"prod explicit", "prod", "https://messaging.bandwidth.com"},
		{"unknown env falls back to prod", "staging", "https://messaging.bandwidth.com"},
		{"test", "test", "https://test.messaging.bandwidth.com"},
		{"uat", "uat", "https://test.messaging.bandwidth.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := messagingHostForEnvironment(tt.env); got != tt.want {
				t.Errorf("messagingHostForEnvironment(%q) = %q, want %q", tt.env, got, tt.want)
			}
		})
	}
}

func TestMessagingHostForEnvironment_BW_MESSAGING_URL(t *testing.T) {
	t.Setenv("BW_MESSAGING_URL", "https://custom.messaging.example.com")
	for _, env := range []string{"", "prod", "test"} {
		got := messagingHostForEnvironment(env)
		if got != "https://custom.messaging.example.com" {
			t.Errorf("messagingHostForEnvironment(%q) with BW_MESSAGING_URL = %q, want override", env, got)
		}
	}
}

func TestMessagingHostForEnvironment_BW_MESSAGING_URL_TrailingSlash(t *testing.T) {
	t.Setenv("BW_MESSAGING_URL", "http://localhost:8080/")
	got := messagingHostForEnvironment("")
	if got != "http://localhost:8080" {
		t.Errorf("messagingHostForEnvironment with trailing slash = %q, want without slash", got)
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
