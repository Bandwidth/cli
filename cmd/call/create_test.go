package call

import (
	"testing"
)

func TestBuildCreateBody(t *testing.T) {
	body := BuildCreateBody(CreateOpts{
		From:      "+19195551234",
		To:        "+15559876543",
		AppID:     "abc-123",
		AnswerURL: "https://example.com/voice",
	})

	if body["from"] != "+19195551234" {
		t.Errorf("from = %q, want +19195551234", body["from"])
	}
	if body["to"] != "+15559876543" {
		t.Errorf("to = %q, want +15559876543", body["to"])
	}
	if body["applicationId"] != "abc-123" {
		t.Errorf("applicationId = %q, want abc-123", body["applicationId"])
	}
	if body["answerUrl"] != "https://example.com/voice" {
		t.Errorf("answerUrl = %q, want https://example.com/voice", body["answerUrl"])
	}
}

func TestExtractCallID(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    string
		wantErr bool
	}{
		{
			name:  "standard callId",
			input: map[string]interface{}{"callId": "c-123-abc"},
			want:  "c-123-abc",
		},
		{
			name:  "id field",
			input: map[string]interface{}{"id": "c-456-def"},
			want:  "c-456-def",
		},
		{
			name:  "CallId field",
			input: map[string]interface{}{"CallId": "c-789-ghi"},
			want:  "c-789-ghi",
		},
		{
			name:    "missing callId",
			input:   map[string]interface{}{"status": "ok"},
			wantErr: true,
		},
		{
			name:    "not a map",
			input:   "just a string",
			wantErr: true,
		},
		{
			name:    "nil",
			input:   nil,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractCallID(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTerminalCallStates(t *testing.T) {
	if !terminalCallStates["disconnected"] {
		t.Error("disconnected should be terminal")
	}
	for _, state := range []string{"queued", "initiated", "answered", "ringing"} {
		if terminalCallStates[state] {
			t.Errorf("%s should not be terminal", state)
		}
	}
}
