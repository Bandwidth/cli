package vcp

import (
	"testing"
)

// --- VCP Create ---

func TestBuildVCPCreateBody_Basic(t *testing.T) {
	body := BuildVCPCreateBody(VCPCreateOpts{
		Name: "Production VCP",
	})
	if body["name"] != "Production VCP" {
		t.Errorf("name = %q, want Production VCP", body["name"])
	}
	if _, ok := body["description"]; ok {
		t.Error("description should not be set when empty")
	}
	if _, ok := body["httpVoiceV2ApplicationId"]; ok {
		t.Error("httpVoiceV2ApplicationId should not be set when empty")
	}
}

func TestBuildVCPCreateBody_WithAllFields(t *testing.T) {
	body := BuildVCPCreateBody(VCPCreateOpts{
		Name:        "Voice VCP",
		Description: "For voice calls",
		AppID:       "abc-123-def",
	})
	if body["name"] != "Voice VCP" {
		t.Errorf("name = %q, want Voice VCP", body["name"])
	}
	if body["description"] != "For voice calls" {
		t.Errorf("description = %q, want For voice calls", body["description"])
	}
	if body["httpVoiceV2ApplicationId"] != "abc-123-def" {
		t.Errorf("httpVoiceV2ApplicationId = %q, want abc-123-def", body["httpVoiceV2ApplicationId"])
	}
}

// --- VCP Update ---

func TestBuildVCPUpdateBody_SingleField(t *testing.T) {
	name := "New Name"
	body, err := BuildVCPUpdateBody(VCPUpdateOpts{Name: &name})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["name"] != "New Name" {
		t.Errorf("name = %q, want New Name", body["name"])
	}
	if len(body) != 1 {
		t.Errorf("expected 1 field, got %d", len(body))
	}
}

func TestBuildVCPUpdateBody_AllFields(t *testing.T) {
	name := "Updated"
	desc := "New description"
	appID := "def-456"
	body, err := BuildVCPUpdateBody(VCPUpdateOpts{
		Name:        &name,
		Description: &desc,
		AppID:       &appID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(body) != 3 {
		t.Errorf("expected 3 fields, got %d", len(body))
	}
	if body["httpVoiceV2ApplicationId"] != "def-456" {
		t.Errorf("httpVoiceV2ApplicationId = %q, want def-456", body["httpVoiceV2ApplicationId"])
	}
}

func TestBuildVCPUpdateBody_NoFields(t *testing.T) {
	_, err := BuildVCPUpdateBody(VCPUpdateOpts{})
	if err == nil {
		t.Fatal("expected error for no fields, got nil")
	}
}

// --- VCP Assign ---

func TestBuildAssignBody(t *testing.T) {
	body := BuildAssignBody([]string{"+19195551234", "+19195551235"})
	if body["action"] != "ADD" {
		t.Errorf("action = %q, want ADD", body["action"])
	}
	numbers, ok := body["phoneNumbers"].([]string)
	if !ok {
		t.Fatal("phoneNumbers is not []string")
	}
	if len(numbers) != 2 {
		t.Errorf("expected 2 numbers, got %d", len(numbers))
	}
	if numbers[0] != "+19195551234" {
		t.Errorf("numbers[0] = %q, want +19195551234", numbers[0])
	}
}

func TestBuildAssignBody_SingleNumber(t *testing.T) {
	body := BuildAssignBody([]string{"+19195551234"})
	numbers := body["phoneNumbers"].([]string)
	if len(numbers) != 1 {
		t.Errorf("expected 1 number, got %d", len(numbers))
	}
}
