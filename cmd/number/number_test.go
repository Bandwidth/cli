package number

import (
	"errors"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
)

func TestBuildOrderBody(t *testing.T) {
	body := BuildOrderBody([]string{"+19195551234", "+19195551235"})

	tnList, ok := body["TelephoneNumberList"].(map[string]interface{})
	if !ok {
		t.Fatal("TelephoneNumberList is not a map")
	}
	numbers, ok := tnList["TelephoneNumber"].([]string)
	if !ok {
		t.Fatal("TelephoneNumber is not []string")
	}
	if len(numbers) != 2 {
		t.Errorf("expected 2 numbers, got %d", len(numbers))
	}
	if numbers[0] != "+19195551234" {
		t.Errorf("numbers[0] = %q, want +19195551234", numbers[0])
	}
	if numbers[1] != "+19195551235" {
		t.Errorf("numbers[1] = %q, want +19195551235", numbers[1])
	}
}

func TestBuildOrderBody_SingleNumber(t *testing.T) {
	body := BuildOrderBody([]string{"+19195551234"})
	tnList := body["TelephoneNumberList"].(map[string]interface{})
	numbers := tnList["TelephoneNumber"].([]string)
	if len(numbers) != 1 {
		t.Errorf("expected 1 number, got %d", len(numbers))
	}
}

func TestNormalizeE164(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"+19195551234", "+19195551234"}, // already E.164
		{"9195551234", "+19195551234"},   // 10-digit US
		{"19195551234", "+19195551234"},  // 11-digit, no +
		{"442071838750", "+442071838750"},
	}
	for _, c := range cases {
		got := normalizeE164(c.in)
		if got != c.want {
			t.Errorf("normalizeE164(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractFullNumbers_MultiResult(t *testing.T) {
	// Shape produced by XMLToMap for /tns with 2 results.
	raw := map[string]interface{}{
		"TelephoneNumbersResponse": map[string]interface{}{
			"TelephoneNumberCount": "2",
			"Links": map[string]interface{}{
				"first": "...",
				"next":  "...",
			},
			"TelephoneNumbers": map[string]interface{}{
				"TelephoneNumber": []interface{}{
					map[string]interface{}{
						"City":       "CARY",
						"FullNumber": "2012381139",
						"Status":     "Inservice",
					},
					map[string]interface{}{
						"City":       "CARY",
						"FullNumber": "+19192381138",
						"Status":     "Inservice",
					},
				},
			},
		},
	}
	got := extractFullNumbers(raw)
	want := map[string]bool{"+12012381139": true, "+19192381138": true}
	if len(got) != len(want) {
		t.Fatalf("got %d numbers, want %d: %v", len(got), len(want), got)
	}
	for _, n := range got {
		if !want[n] {
			t.Errorf("unexpected number %q", n)
		}
	}
}

func TestExtractFullNumbers_SingleResult(t *testing.T) {
	// XMLToMap produces a single map (not array) when only one result.
	raw := map[string]interface{}{
		"TelephoneNumbersResponse": map[string]interface{}{
			"TelephoneNumberCount": "1",
			"TelephoneNumbers": map[string]interface{}{
				"TelephoneNumber": map[string]interface{}{
					"City":       "CARY",
					"FullNumber": "2012381139",
					"Status":     "Inservice",
				},
			},
		},
	}
	got := extractFullNumbers(raw)
	if len(got) != 1 || got[0] != "+12012381139" {
		t.Errorf("got %v, want [+12012381139]", got)
	}
}

func TestExtractFullNumbers_Empty(t *testing.T) {
	raw := map[string]interface{}{
		"TelephoneNumbersResponse": map[string]interface{}{
			"TelephoneNumberCount": "0",
		},
	}
	got := extractFullNumbers(raw)
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestWrapTNsError_403_NonBuild(t *testing.T) {
	apiErr := &api.APIError{StatusCode: 403, Body: ""}
	err := wrapTNsError(apiErr, "9901409", false)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "9901409") {
		t.Errorf("error should name the account id, got %q", msg)
	}
	if !strings.Contains(msg, "Numbers role") {
		t.Errorf("error should mention missing role, got %q", msg)
	}
	// Must preserve the underlying APIError for exit-code mapping.
	var unwrapped *api.APIError
	if !errors.As(err, &unwrapped) || unwrapped.StatusCode != 403 {
		t.Errorf("wrapped error should unwrap to APIError 403")
	}
}

func TestWrapTNsError_403_Build(t *testing.T) {
	apiErr := &api.APIError{StatusCode: 403, Body: ""}
	err := wrapTNsError(apiErr, "9901409", true)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Bandwidth Build") {
		t.Errorf("Build-account 403 message should reference Bandwidth Build, got %q", msg)
	}
	if strings.Contains(msg, "Numbers role") {
		t.Errorf("Build-account message should not point users at the Numbers role, got %q", msg)
	}
	// Must preserve the underlying APIError so ExitCodeForError can read it.
	var unwrapped *api.APIError
	if !errors.As(err, &unwrapped) || unwrapped.StatusCode != 403 {
		t.Errorf("wrapped error should unwrap to APIError 403")
	}
}

func TestWrapTNsError_NonAPIError(t *testing.T) {
	err := wrapTNsError(errors.New("network down"), "9901409", false)
	if !strings.Contains(err.Error(), "network down") {
		t.Errorf("should pass through non-API error, got %q", err.Error())
	}
}

func TestWrapTNsError_500(t *testing.T) {
	// Non-403 API errors should pass through without the 403-specific message.
	apiErr := &api.APIError{StatusCode: 500, Body: "server broke"}
	err := wrapTNsError(apiErr, "9901409", false)
	if strings.Contains(err.Error(), "Numbers role") {
		t.Errorf("500 should not get the 403 message, got %q", err.Error())
	}
}
