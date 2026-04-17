package cmdutil

import (
	"testing"
)

func TestValidateID_Valid(t *testing.T) {
	valid := []string{
		"abc-123",
		"d27b5ce6-167f-4664-9c03-c60b472f6fae",
		"152681",
		"some_id",
		"c-8605e2ca-022696ef",
	}
	for _, id := range valid {
		if err := ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) returned unexpected error: %v", id, err)
		}
	}
}

func TestValidateID_Empty(t *testing.T) {
	err := ValidateID("")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
	if err.Error() != "ID must not be empty" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateID_ForbiddenChars(t *testing.T) {
	forbidden := []string{
		"abc/def",
		"id?param=1",
		"id&other",
		"id#frag",
		"id%20encoded",
	}
	for _, id := range forbidden {
		err := ValidateID(id)
		if err == nil {
			t.Errorf("ValidateID(%q) should have returned error", id)
		}
	}
}

func TestValidateID_Whitespace(t *testing.T) {
	whitespace := []string{
		"id with space",
		"id\twith\ttab",
		"id\nwith\nnewline",
		"id\rwith\rreturn",
	}
	for _, id := range whitespace {
		err := ValidateID(id)
		if err == nil {
			t.Errorf("ValidateID(%q) should have returned error for whitespace", id)
		}
	}
}
