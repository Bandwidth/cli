package app

import (
	"testing"
)

func TestValidateCreateOpts(t *testing.T) {
	tests := []struct {
		name    string
		opts    CreateOpts
		wantErr bool
	}{
		{
			name:    "valid voice",
			opts:    CreateOpts{Name: "My App", Type: "voice", CallbackURL: "https://example.com"},
			wantErr: false,
		},
		{
			name:    "valid messaging",
			opts:    CreateOpts{Name: "My App", Type: "messaging", CallbackURL: "https://example.com"},
			wantErr: false,
		},
		{
			name:    "invalid type",
			opts:    CreateOpts{Name: "My App", Type: "sms", CallbackURL: "https://example.com"},
			wantErr: true,
		},
		{
			name:    "empty type",
			opts:    CreateOpts{Name: "My App", Type: "", CallbackURL: "https://example.com"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCreateOpts(tc.opts)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuildCreateBody(t *testing.T) {
	t.Run("voice app", func(t *testing.T) {
		body := BuildCreateBody(CreateOpts{
			Name:        "Voice App",
			Type:        "voice",
			CallbackURL: "https://example.com/voice",
		})
		if body["ServiceType"] != "Voice-V2" {
			t.Errorf("ServiceType = %q, want Voice-V2", body["ServiceType"])
		}
		if body["AppName"] != "Voice App" {
			t.Errorf("AppName = %q, want Voice App", body["AppName"])
		}
		if body["CallInitiatedCallbackUrl"] != "https://example.com/voice" {
			t.Errorf("CallInitiatedCallbackUrl = %q, want https://example.com/voice", body["CallInitiatedCallbackUrl"])
		}
	})

	t.Run("messaging app", func(t *testing.T) {
		body := BuildCreateBody(CreateOpts{
			Name:        "SMS App",
			Type:        "messaging",
			CallbackURL: "https://example.com/sms",
		})
		if body["ServiceType"] != "Messaging-V2" {
			t.Errorf("ServiceType = %q, want Messaging-V2", body["ServiceType"])
		}
	})
}
