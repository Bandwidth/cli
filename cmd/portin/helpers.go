package portin

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
)

// stripE164 converts "+19195551234" to "9195551234" for the Dashboard API,
// which expects bare 10-digit numbers in request bodies.
func stripE164(number string) string {
	n := strings.TrimPrefix(number, "+")
	if len(n) == 11 && strings.HasPrefix(n, "1") {
		return n[1:]
	}
	return n
}

// digString recursively searches a parsed XML response for the first
// occurrence of key and returns its string value. Returns "" if not found
// or if the value isn't a string.
func digString(v interface{}, key string) string {
	switch val := v.(type) {
	case map[string]interface{}:
		if s, ok := val[key]; ok {
			if str, ok := s.(string); ok {
				return str
			}
		}
		for _, child := range val {
			if found := digString(child, key); found != "" {
				return found
			}
		}
	case []interface{}:
		for _, item := range val {
			if found := digString(item, key); found != "" {
				return found
			}
		}
	}
	return ""
}

// digAllStrings recursively collects every string value at the given key.
// Used for things like extracting all error codes or all TN entries from a
// nested response.
func digAllStrings(v interface{}, key string, out *[]string) {
	switch val := v.(type) {
	case map[string]interface{}:
		if s, ok := val[key]; ok {
			collectStrings(s, out)
		}
		for _, child := range val {
			digAllStrings(child, key, out)
		}
	case []interface{}:
		for _, item := range val {
			digAllStrings(item, key, out)
		}
	}
}

func collectStrings(v interface{}, out *[]string) {
	switch s := v.(type) {
	case string:
		*out = append(*out, s)
	case []interface{}:
		for _, item := range s {
			collectStrings(item, out)
		}
	}
}

// is7300 reports whether the parsed response carries error code 7300, which
// indicates a supp PUT was accepted by the API but never propagated to
// Neustar (typically wireless_to_wireless after FOC, or a state where supps
// are blocked). The user's change has not taken effect.
//
// Reference: Confluence DEVQ/4501996275 — supp returns 7300 on subsequent
// GET when wireless_to_wireless and the order is past FOC.
func is7300(result interface{}) bool {
	codes := []string{}
	digAllStrings(result, "Code", &codes)
	for _, c := range codes {
		if strings.TrimSpace(c) == "7300" {
			return true
		}
	}
	return false
}

// flattenPortInResult collapses the nested XML port-in response into the v1
// stable plain shape: a flat object with orderId/status/focDate/numbers/
// customerOrderId/errorCode keys. Missing fields default to "" or [].
//
// fallbackOrderID is used when the API response itself does not carry the
// OrderId — most notably the GET endpoint, where the ID is part of the URL
// rather than the body. Pass "" if you do not have a fallback.
func flattenPortInResult(result interface{}, fallbackOrderID string) map[string]interface{} {
	numbers := []string{}
	digAllStrings(result, "PhoneNumber", &numbers)
	for i, n := range numbers {
		numbers[i] = cmdutil.NormalizeNumber(n)
	}

	orderID := digString(result, "OrderId")
	if orderID == "" {
		orderID = fallbackOrderID
	}

	return map[string]interface{}{
		"orderId":         orderID,
		"status":          digString(result, "ProcessingStatus"),
		"focDate":         digString(result, "RequestedFocDate"),
		"numbers":         numbers,
		"customerOrderId": digString(result, "CustomerOrderId"),
		"errorCode":       extractErrorCode(result),
	}
}

// extractErrorCode pulls a real error code out of an Errors/ErrorList block.
// HTTP success codes (e.g. 201 in <Status><Code>201</Code></Status>) are
// explicitly ignored — those aren't errors. Returns "" when there is no
// error-bearing code.
func extractErrorCode(result interface{}) string {
	codes := []string{}
	collectErrorCodes(result, &codes)
	for _, c := range codes {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		// HTTP 2xx codes that show up under <Status> are not real errors.
		if len(c) == 3 && c[0] == '2' {
			continue
		}
		return c
	}
	return ""
}

// collectErrorCodes recurses, but only into keys whose names suggest an
// error-bearing block (Errors, ErrorList, Error). This avoids capturing
// the Code in <Status><Code>201</Code></Status> which is HTTP success
// metadata, not an error.
func collectErrorCodes(v interface{}, out *[]string) {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			lk := strings.ToLower(k)
			if lk == "error" || lk == "errors" || lk == "errorlist" {
				digAllStrings(child, "Code", out)
				continue
			}
			collectErrorCodes(child, out)
		}
	case []interface{}:
		for _, item := range val {
			collectErrorCodes(item, out)
		}
	}
}

// portinError wraps API errors with context-appropriate messaging for the
// porting endpoints. Maps known role/feature failures to FeatureLimitError
// so they exit 4 instead of generic 1.
//
// Toll-free Phase 1 not enabled: surface as exit 4 with the documented
// upgrade path. The API returns a 403 in this case (same shape as a missing
// role); we cannot distinguish without inspecting the response body, so the
// message lists both possibilities.
func portinError(err error, op string) error {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%s: %w", op, err)
	}
	switch apiErr.StatusCode {
	case 403:
		body := strings.ToLower(apiErr.Body)
		if strings.Contains(body, "toll_free") || strings.Contains(body, "tollfree") || strings.Contains(body, "phase_1") || strings.Contains(body, "phase 1") {
			return cmdutil.NewFeatureLimit(
				"toll-free port-ins via the API require Phase 1 automation, which is not enabled on your account.\n"+
					"Contact your Bandwidth account manager. Numbers must otherwise be ported through the Bandwidth Dashboard or Operations.",
				err,
			)
		}
		return cmdutil.Wrap403(err, op, "Number Management")
	case 404:
		return fmt.Errorf("%s: order not found", op)
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}
