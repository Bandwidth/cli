package output

import "strings"

// NormalizeToArray ensures list command output is always an array.
// The XML-to-JSON parser returns a single object when there's one result,
// which causes silent agent failures. This wraps a lone map in a slice.
func NormalizeToArray(data interface{}) interface{} {
	switch v := data.(type) {
	case []interface{}:
		return v
	case map[string]interface{}:
		return []interface{}{v}
	default:
		return data
	}
}

// metadataKeys are keys that indicate a wrapper object, not a resource.
var metadataKeys = map[string]bool{
	"Count": true, "TotalCount": true, "ResultCount": true,
	"Links": true, "first": true, "last": true, "next": true, "prev": true,
}

// FlattenResponse recursively unwraps single-key objects until it reaches
// an array or a meaningful multi-key object. For wrapper objects (metadata + one array),
// it extracts the array. This converts structures like
// {"TNs":{"TelephoneNumbers":{"Count":"5","TelephoneNumber":[...]}}} into just [...].
func FlattenResponse(data interface{}) interface{} {
	m, ok := data.(map[string]interface{})
	if !ok {
		return data
	}

	// Single-key wrapper — unwrap and recurse.
	if len(m) == 1 {
		for _, v := range m {
			return FlattenResponse(v)
		}
	}

	// JSON API envelope: {data: ..., links: [...], errors: [...], page: {...}}
	// Extract just the "data" field if the other keys are standard envelope keys.
	if d, hasData := m["data"]; hasData {
		isEnvelope := true
		for k := range m {
			if k != "data" && k != "links" && k != "errors" && k != "page" {
				isEnvelope = false
				break
			}
		}
		if isEnvelope {
			return FlattenResponse(d)
		}
	}

	// Recurse into values first to flatten nested wrappers.
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = FlattenResponse(v)
	}

	// Check if this looks like a list wrapper: one array + remaining keys are
	// all metadata (Count, Links, etc.) with string values.
	if arr, ok := extractListArray(result); ok {
		return arr
	}

	return result
}

// extractListArray checks if a map is a "list wrapper" — one array value
// and all other keys are known metadata keys with string values.
func extractListArray(m map[string]interface{}) (interface{}, bool) {
	var arrayVal interface{}
	arrayCount := 0

	for k, v := range m {
		switch v.(type) {
		case []interface{}:
			arrayVal = v
			arrayCount++
		case string:
			if !metadataKeys[k] && !strings.Contains(k, "Count") && !strings.Contains(k, "Link") {
				return nil, false // non-metadata string field → this is a resource, not a wrapper
			}
		default:
			return nil, false // complex non-array value → not a simple wrapper
		}
	}

	if arrayCount == 1 {
		return arrayVal, true
	}
	return nil, false
}
