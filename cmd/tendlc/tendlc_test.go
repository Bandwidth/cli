package tendlc

import (
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

func TestExtractData(t *testing.T) {
	t.Run("standard paginated response", func(t *testing.T) {
		resp := map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{
					"phoneNumber": "+12054443942",
					"campaignId":  "CA3XKE1",
					"status":      "SUCCESS",
				},
			},
			"page": map[string]interface{}{
				"totalElements": float64(1),
			},
		}
		data := extractData(resp)
		arr, ok := data.([]interface{})
		if !ok {
			t.Fatalf("expected []interface{}, got %T", data)
		}
		if len(arr) != 1 {
			t.Fatalf("expected 1 element, got %d", len(arr))
		}
	})

	t.Run("no data key", func(t *testing.T) {
		resp := map[string]interface{}{
			"something": "else",
		}
		data := extractData(resp)
		m, ok := data.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map, got %T", data)
		}
		if m["something"] != "else" {
			t.Error("expected original response returned as-is")
		}
	})

	t.Run("non-map response", func(t *testing.T) {
		resp := "just a string"
		data := extractData(resp)
		if data != resp {
			t.Error("expected passthrough for non-map input")
		}
	})

	t.Run("nil response", func(t *testing.T) {
		data := extractData(nil)
		if data != nil {
			t.Errorf("expected nil, got %v", data)
		}
	})

	t.Run("empty data array", func(t *testing.T) {
		resp := map[string]interface{}{
			"data": []interface{}{},
			"page": map[string]interface{}{
				"totalElements": float64(0),
			},
		}
		data := extractData(resp)
		arr, ok := data.([]interface{})
		if !ok {
			t.Fatalf("expected []interface{}, got %T", data)
		}
		if len(arr) != 0 {
			t.Errorf("expected empty array, got %d elements", len(arr))
		}
	})
}

func TestFilterNumbers(t *testing.T) {
	numbers := []interface{}{
		map[string]interface{}{"phoneNumber": "+11111111111", "status": "SUCCESS", "campaignId": "C1"},
		map[string]interface{}{"phoneNumber": "+12222222222", "status": "FAILURE", "campaignId": "C1"},
		map[string]interface{}{"phoneNumber": "+13333333333", "status": "SUCCESS", "campaignId": "C2"},
		map[string]interface{}{"phoneNumber": "+14444444444", "status": "PROCESSING"},
	}

	t.Run("filter by status", func(t *testing.T) {
		result := filterNumbers(numbers, "FAILURE", "")
		arr := result.([]interface{})
		if len(arr) != 1 {
			t.Fatalf("expected 1, got %d", len(arr))
		}
		m := arr[0].(map[string]interface{})
		if m["phoneNumber"] != "+12222222222" {
			t.Errorf("got %v", m["phoneNumber"])
		}
	})

	t.Run("filter by campaign", func(t *testing.T) {
		result := filterNumbers(numbers, "", "C2")
		arr := result.([]interface{})
		if len(arr) != 1 {
			t.Fatalf("expected 1, got %d", len(arr))
		}
	})

	t.Run("filter by both", func(t *testing.T) {
		result := filterNumbers(numbers, "SUCCESS", "C1")
		arr := result.([]interface{})
		if len(arr) != 1 {
			t.Fatalf("expected 1, got %d", len(arr))
		}
		m := arr[0].(map[string]interface{})
		if m["phoneNumber"] != "+11111111111" {
			t.Errorf("got %v", m["phoneNumber"])
		}
	})

	t.Run("no matches returns empty array", func(t *testing.T) {
		result := filterNumbers(numbers, "FAILURE", "C999")
		arr := result.([]interface{})
		if len(arr) != 0 {
			t.Errorf("expected 0, got %d", len(arr))
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		result := filterNumbers(numbers, "success", "c1")
		arr := result.([]interface{})
		if len(arr) != 1 {
			t.Fatalf("expected 1, got %d", len(arr))
		}
	})

	t.Run("non-array passthrough", func(t *testing.T) {
		result := filterNumbers("not an array", "SUCCESS", "")
		if result != "not an array" {
			t.Error("expected passthrough")
		}
	})
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
