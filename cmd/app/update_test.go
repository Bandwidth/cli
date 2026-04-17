package app

import (
	"testing"
)

func TestDetectAppType(t *testing.T) {
	tests := []struct {
		name string
		app  interface{}
		want string
	}{
		{
			name: "messaging app",
			app: map[string]interface{}{
				"Application": map[string]interface{}{
					"ServiceType": "Messaging-V2",
					"AppName":     "My SMS App",
				},
			},
			want: "messaging",
		},
		{
			name: "voice app",
			app: map[string]interface{}{
				"Application": map[string]interface{}{
					"ServiceType": "Voice-V2",
					"AppName":     "My Voice App",
				},
			},
			want: "voice",
		},
		{
			name: "flat response",
			app: map[string]interface{}{
				"ServiceType": "Messaging-V2",
			},
			want: "messaging",
		},
		{
			name: "not a map",
			app:  "just a string",
			want: "voice", // defaults to voice
		},
		{
			name: "nil",
			app:  nil,
			want: "voice",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectAppType(tc.app)
			if got != tc.want {
				t.Errorf("detectAppType() = %q, want %q", got, tc.want)
			}
		})
	}
}
