package message

import (
	"testing"
)

func TestValidateSendOpts(t *testing.T) {
	tests := []struct {
		name    string
		opts    SendOpts
		wantErr bool
	}{
		{
			name: "valid SMS",
			opts: SendOpts{
				To: []string{"+15551234567"}, From: "+15559876543",
				AppID: "abc-123", Text: "Hello",
			},
		},
		{
			name: "valid MMS media only",
			opts: SendOpts{
				To: []string{"+15551234567"}, From: "+15559876543",
				AppID: "abc-123", Media: []string{"https://example.com/img.png"},
			},
		},
		{
			name: "valid with text and media",
			opts: SendOpts{
				To: []string{"+15551234567"}, From: "+15559876543",
				AppID: "abc-123", Text: "Look at this", Media: []string{"https://example.com/img.png"},
			},
		},
		{
			name: "no text and no media",
			opts: SendOpts{
				To: []string{"+15551234567"}, From: "+15559876543",
				AppID: "abc-123",
			},
			wantErr: true,
		},
		{
			name: "invalid priority",
			opts: SendOpts{
				To: []string{"+15551234567"}, From: "+15559876543",
				AppID: "abc-123", Text: "Hello", Priority: "urgent",
			},
			wantErr: true,
		},
		{
			name: "valid priority default",
			opts: SendOpts{
				To: []string{"+15551234567"}, From: "+15559876543",
				AppID: "abc-123", Text: "Hello", Priority: "default",
			},
		},
		{
			name: "valid priority high",
			opts: SendOpts{
				To: []string{"+15551234567"}, From: "+15559876543",
				AppID: "abc-123", Text: "Hello", Priority: "high",
			},
		},
		{
			name: "empty priority is valid",
			opts: SendOpts{
				To: []string{"+15551234567"}, From: "+15559876543",
				AppID: "abc-123", Text: "Hello",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSendOpts(tc.opts)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuildSendBody(t *testing.T) {
	t.Run("minimal SMS", func(t *testing.T) {
		body := BuildSendBody(SendOpts{
			To:    []string{"+15551234567"},
			From:  "+15559876543",
			AppID: "abc-123",
			Text:  "Hello world",
		})

		if body["from"] != "+15559876543" {
			t.Errorf("from = %q, want +15559876543", body["from"])
		}
		if body["applicationId"] != "abc-123" {
			t.Errorf("applicationId = %q, want abc-123", body["applicationId"])
		}
		if body["text"] != "Hello world" {
			t.Errorf("text = %q, want Hello world", body["text"])
		}
		to, ok := body["to"].([]string)
		if !ok || len(to) != 1 || to[0] != "+15551234567" {
			t.Errorf("to = %v, want [+15551234567]", body["to"])
		}
		// Optional fields should be absent
		if _, ok := body["media"]; ok {
			t.Error("media should not be present for SMS")
		}
		if _, ok := body["tag"]; ok {
			t.Error("tag should not be present when empty")
		}
		if _, ok := body["priority"]; ok {
			t.Error("priority should not be present when empty")
		}
		if _, ok := body["expiration"]; ok {
			t.Error("expiration should not be present when empty")
		}
	})

	t.Run("MMS with media only", func(t *testing.T) {
		body := BuildSendBody(SendOpts{
			To:    []string{"+15551234567"},
			From:  "+15559876543",
			AppID: "abc-123",
			Media: []string{"https://example.com/image.png"},
		})

		media, ok := body["media"].([]string)
		if !ok || len(media) != 1 || media[0] != "https://example.com/image.png" {
			t.Errorf("media = %v, want [https://example.com/image.png]", body["media"])
		}
		if _, ok := body["text"]; ok {
			t.Error("text should not be present when empty")
		}
	})

	t.Run("MMS with multiple media", func(t *testing.T) {
		body := BuildSendBody(SendOpts{
			To:    []string{"+15551234567"},
			From:  "+15559876543",
			AppID: "abc-123",
			Text:  "Multiple attachments",
			Media: []string{"https://example.com/a.png", "https://example.com/b.jpg"},
		})

		media, ok := body["media"].([]string)
		if !ok || len(media) != 2 {
			t.Errorf("expected 2 media URLs, got %v", body["media"])
		}
	})

	t.Run("group message", func(t *testing.T) {
		body := BuildSendBody(SendOpts{
			To:    []string{"+15551234567", "+15552345678", "+15553456789"},
			From:  "+15559876543",
			AppID: "abc-123",
			Text:  "Hey everyone",
		})

		to, ok := body["to"].([]string)
		if !ok || len(to) != 3 {
			t.Errorf("to should have 3 recipients, got %v", body["to"])
		}
	})

	t.Run("all optional fields", func(t *testing.T) {
		body := BuildSendBody(SendOpts{
			To:         []string{"+15551234567"},
			From:       "+15559876543",
			AppID:      "abc-123",
			Text:       "Hello",
			Media:      []string{"https://example.com/img.png"},
			Tag:        "my-tag",
			Priority:   "high",
			Expiration: "2025-01-01T00:00:00Z",
		})

		if body["tag"] != "my-tag" {
			t.Errorf("tag = %q, want my-tag", body["tag"])
		}
		if body["priority"] != "high" {
			t.Errorf("priority = %q, want high", body["priority"])
		}
		if body["expiration"] != "2025-01-01T00:00:00Z" {
			t.Errorf("expiration = %q, want 2025-01-01T00:00:00Z", body["expiration"])
		}
	})

	t.Run("required fields always present", func(t *testing.T) {
		body := BuildSendBody(SendOpts{
			To:    []string{"+15551234567"},
			From:  "+15559876543",
			AppID: "abc-123",
			Text:  "test",
		})

		for _, key := range []string{"to", "from", "applicationId"} {
			if _, ok := body[key]; !ok {
				t.Errorf("required field %q missing from body", key)
			}
		}
	})
}
