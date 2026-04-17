package output

// FindByName searches a flattened list response for an item where the given
// field matches name. Returns the item if found, nil otherwise.
// Handles both array results and single-object results from the XML-to-JSON parser.
func FindByName(data any, field, name string) any {
	flat := FlattenResponse(data)
	switch v := flat.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if val, ok := m[field].(string); ok && val == name {
					return item
				}
			}
		}
	case map[string]any:
		if val, ok := v[field].(string); ok && val == name {
			return v
		}
	}
	return nil
}
