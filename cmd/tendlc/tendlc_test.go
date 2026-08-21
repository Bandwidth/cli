package tendlc

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
)

func TestRoleGateError_RegistrationCenter(t *testing.T) {
	err := roleGateError(&api.APIError{
		StatusCode: 403,
		Body:       `{"errors":[{"type":"forbidden","description":"Account 33333 is not enabled for the Registration Center"}]}`,
	}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "not enabled for the Registration Center") {
		t.Errorf("got %q, want Registration Center message", got)
	}
}

func TestRoleGateError_ImportCustomer(t *testing.T) {
	err := roleGateError(&api.APIError{
		StatusCode: 403,
		Body:       `{"errors":[{"type":"forbidden","description":"'10DLC campaign management' import customer is not enabled on account 33333"}]}`,
	}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "register campaigns through TCR") {
		t.Errorf("got %q, want import customer message", got)
	}
}

func TestRoleGateError_FeatureNotEnabled(t *testing.T) {
	err := roleGateError(&api.APIError{
		StatusCode: 403,
		Body:       `{"errors":[{"type":"forbidden","description":"'10DLC campaign management' is not enabled on account 33333"}]}`,
	}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "campaign management is not enabled") {
		t.Errorf("got %q, want feature not enabled message", got)
	}
}

func TestRoleGateError_NoRole(t *testing.T) {
	err := roleGateError(&api.APIError{
		StatusCode: 403,
		Body:       `{"errors":[{"type":"forbidden","description":"client does not have access rights to the content"}]}`,
	}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "Campaign Management role") {
		t.Errorf("got %q, want role message", got)
	}
}

func TestRoleGateError_UnknownBody(t *testing.T) {
	err := roleGateError(&api.APIError{StatusCode: 403, Body: "something unexpected"}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "access denied (403)") {
		t.Errorf("got %q, want fallback message", got)
	}
}

func TestRoleGateError_OtherStatus(t *testing.T) {
	err := roleGateError(&api.APIError{StatusCode: 500, Body: "Internal Server Error"}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "API request failed") {
		t.Errorf("got %q, want API request failed", got)
	}
}

func TestRoleGateError_NonAPIError(t *testing.T) {
	err := roleGateError(&api.APIError{StatusCode: 404, Body: "Not Found"}, "TFV")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStatusCommandRegistered(t *testing.T) {
	c, _, err := Cmd.Find([]string{"status"})
	if err != nil || c.Name() != "status" {
		t.Fatalf("Find(status) = %v, err %v; want the status command", c, err)
	}
}

func TestStatusResultShape(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantAccess string
		wantReason string
	}{
		{"success", 200, `{"data":[]}`, "available", "probe_succeeded"},
		{"role missing", 403, `{"errors":[{"description":"does not have access rights"}]}`,
			"unavailable", "role_absent"},
		{"registration center off", 403, `{"errors":[{"description":"not enabled for the Registration Center"}]}`,
			"unavailable", "registration_center_not_enabled"},
		{"campaign mgmt off", 403, `{"errors":[{"description":"10DLC is not enabled on account"}]}`,
			"unavailable", "campaign_management_not_enabled"},
		{"unrecognized 403", 403, `{"errors":[{"description":"something new"}]}`,
			"unavailable", "access_denied"},
		{"server error", 503, `{}`, "unknown", "probe_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusResult(tt.statusCode, tt.body)
			if got["access"] != tt.wantAccess {
				t.Errorf("access = %v, want %v", got["access"], tt.wantAccess)
			}
			if got["reason"] != tt.wantReason {
				t.Errorf("reason = %v, want %v", got["reason"], tt.wantReason)
			}
		})
	}
}

// mode is always present and always unknown — an omitted field invites
// callers to guess a default.
func TestStatusAlwaysReportsModeUnknown(t *testing.T) {
	for _, code := range []int{200, 403, 503} {
		got := statusResult(code, `{}`)
		mode, ok := got["mode"].(map[string]string)
		if !ok {
			t.Fatalf("code %d: mode missing or wrong type: %#v", code, got["mode"])
		}
		if mode["status"] != "unknown" || mode["reason"] != "not_discoverable" {
			t.Errorf("code %d: mode = %v, want unknown/not_discoverable", code, mode)
		}
	}
}

// isNotFound is the sole translator of a real 404 into pollTarget's
// found=false, which is the mechanism the create-vs-delete GoneIsDone
// contract in async.go depends on — it must recognize a 404 wherever it
// appears in the error chain, and reject everything else, including a nil
// error.
func TestIsNotFound(t *testing.T) {
	notFound := &api.APIError{StatusCode: 404, Body: "brand not found"}
	serverError := &api.APIError{StatusCode: 500, Body: "boom"}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"404 API error", notFound, true},
		{"500 API error", serverError, false},
		{"wrapped 404", fmt.Errorf("fetching brand: %w", notFound), true},
		{"plain non-API error", errors.New("connection reset"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNotFound(tt.err); got != tt.want {
				t.Errorf("isNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
