package cmdutil

import (
	"fmt"
	"strings"
)

// ValidateID checks that a user-supplied ID does not contain characters that
// could inject path segments or query parameters into a URL.  The forbidden
// set covers the most common path-traversal and query-injection characters:
// slash, question mark, ampersand, hash, percent, and any ASCII whitespace.
func ValidateID(id string) error {
	const forbidden = "/?&#%"
	if strings.ContainsAny(id, forbidden) {
		return fmt.Errorf("invalid ID %q: must not contain '/', '?', '&', '#', or '%%'", id)
	}
	for _, r := range id {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return fmt.Errorf("invalid ID %q: must not contain whitespace", id)
		}
	}
	if id == "" {
		return fmt.Errorf("ID must not be empty")
	}
	return nil
}
