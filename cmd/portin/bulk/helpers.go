package bulk

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
)

// stripE164 mirrors cmd/portin/helpers.go since that helper is package-private.
// Removes the leading + and a US/CA country code prefix.
func stripE164(number string) string {
	n := strings.TrimPrefix(number, "+")
	if len(n) == 11 && strings.HasPrefix(n, "1") {
		return n[1:]
	}
	return n
}

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

// flattenBulkResult collapses the nested response into the v1 plain shape:
// { bulkOrderId, status, childOrderIds, portableNumbers, nonPortable }.
func flattenBulkResult(result interface{}) map[string]interface{} {
	childIDs := []string{}
	digAllStrings(result, "OrderId", &childIDs)
	// First OrderId is the bulk order itself; the rest are children when
	// they appear inside ChildPortinOrder/ChildPortinOrderList nodes.
	bulkID := digString(result, "OrderId")
	// Filter the bulk ID out of children if it appears alongside.
	filteredChildren := []string{}
	for _, id := range childIDs {
		if id != bulkID {
			filteredChildren = append(filteredChildren, id)
		}
	}

	portable := []string{}
	digAllStrings(result, "TollFreeNumber", &portable)
	tnPortable := []string{}
	digAllStrings(result, "TN", &tnPortable)
	portable = append(portable, tnPortable...)
	for i, p := range portable {
		portable[i] = cmdutil.NormalizeNumber(p)
	}

	// Non-portable: dig errors with a TnList payload.
	nonPortable := []map[string]interface{}{}
	collectErrorEntries(result, &nonPortable)

	return map[string]interface{}{
		"bulkOrderId":     bulkID,
		"status":          digString(result, "ProcessingStatus"),
		"childOrderIds":   filteredChildren,
		"portableNumbers": portable,
		"nonPortable":     nonPortable,
	}
}

// collectErrorEntries finds all Error nodes and pulls their Code, Description,
// and contained TNs into a flat list.
func collectErrorEntries(v interface{}, out *[]map[string]interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		if _, hasCode := val["Code"]; hasCode {
			tns := []string{}
			digAllStrings(val, "TN", &tns)
			digAllStrings(val, "Tn", &tns)
			for _, tn := range tns {
				*out = append(*out, map[string]interface{}{
					"number": cmdutil.NormalizeNumber(tn),
					"code":   digString(val, "Code"),
					"reason": digString(val, "Description"),
				})
			}
			return
		}
		for _, child := range val {
			collectErrorEntries(child, out)
		}
	case []interface{}:
		for _, item := range val {
			collectErrorEntries(item, out)
		}
	}
}

// bulkError mirrors cmd/portin/helpers.go portinError but for bulk endpoints.
func bulkError(err error, op string) error {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%s: %w", op, err)
	}
	switch apiErr.StatusCode {
	case 403:
		return cmdutil.Wrap403(err, op, "Number Management")
	case 404:
		return fmt.Errorf("%s: bulk order not found", op)
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}
