package account

import "testing"

func TestNormalizeDeliveryChannel(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "lowercase sms", raw: "sms", want: "SMS"},
		{name: "lowercase voice", raw: "voice", want: "VOICE"},
		{name: "uppercase already", raw: "SMS", want: "SMS"},
		{name: "mixed case with whitespace", raw: "  Voice  ", want: "VOICE"},
		{name: "invalid value", raw: "email", wantErr: true},
		{name: "empty value", raw: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDeliveryChannel(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeDeliveryChannel(%q) = %q, want error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeDeliveryChannel(%q) unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("normalizeDeliveryChannel(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
