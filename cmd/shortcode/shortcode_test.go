package shortcode

import (
	"testing"

	"github.com/Bandwidth/cli/internal/api"
)

func TestShortcodeError_403(t *testing.T) {
	err := shortcodeError(&api.APIError{StatusCode: 403, Body: "Forbidden"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	got := err.Error()
	if !contains(got, "short code access") {
		t.Errorf("got %q, want it to mention short code access", got)
	}
}

func TestShortcodeError_Other(t *testing.T) {
	err := shortcodeError(&api.APIError{StatusCode: 500, Body: "Internal Server Error"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	got := err.Error()
	if !contains(got, "listing short codes") {
		t.Errorf("got %q, want it to contain 'listing short codes'", got)
	}
}

func TestExtractData(t *testing.T) {
	t.Run("standard response", func(t *testing.T) {
		resp := map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{"shortCode": "12345", "status": "ACTIVE"},
			},
			"page": map[string]interface{}{"totalElements": float64(1)},
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

	t.Run("empty data", func(t *testing.T) {
		resp := map[string]interface{}{
			"data": []interface{}{},
		}
		data := extractData(resp)
		arr, ok := data.([]interface{})
		if !ok {
			t.Fatalf("expected []interface{}, got %T", data)
		}
		if len(arr) != 0 {
			t.Errorf("expected 0 elements, got %d", len(arr))
		}
	})

	t.Run("non-map passthrough", func(t *testing.T) {
		data := extractData("not a map")
		if data != "not a map" {
			t.Error("expected passthrough")
		}
	})
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
