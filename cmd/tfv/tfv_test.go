package tfv

import (
	"fmt"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
)

func TestTfvError_403(t *testing.T) {
	err := tfvError(&api.APIError{StatusCode: 403, Body: "Forbidden"}, "+18005551234")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	got := err.Error()
	if !contains(got, "TFV role") {
		t.Errorf("got %q, want it to mention TFV role", got)
	}
}

func TestTfvError_404(t *testing.T) {
	err := tfvError(&api.APIError{StatusCode: 404, Body: "Not Found"}, "+18005551234")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	got := err.Error()
	if !contains(got, "band tfv submit") {
		t.Errorf("got %q, want it to suggest band tfv submit", got)
	}
	if !contains(got, "+18005551234") {
		t.Errorf("got %q, want it to include the phone number", got)
	}
}

func TestTfvError_OtherStatus(t *testing.T) {
	err := tfvError(&api.APIError{StatusCode: 500, Body: "Internal Server Error"}, "+18005551234")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	got := err.Error()
	if !contains(got, "checking verification") {
		t.Errorf("got %q, want it to contain 'checking verification'", got)
	}
}

func TestTfvError_NonAPIError(t *testing.T) {
	err := tfvError(fmt.Errorf("connection refused"), "+18005551234")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	got := err.Error()
	if !contains(got, "connection refused") {
		t.Errorf("got %q, want it to wrap the original error", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
