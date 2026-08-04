package cmdutil

import "strings"

// NumberType represents the type of a phone number for messaging purposes.
type NumberType int

const (
	NumberTypeUnknown   NumberType = iota
	NumberType10DLC                // Local 10-digit US/CA number
	NumberTypeTollFree             // US/CA toll-free (800, 888, 877, 866, 855, 844, 833)
	NumberTypeShortCode            // 5-6 digit short code
)

func (t NumberType) String() string {
	switch t {
	case NumberType10DLC:
		return "10DLC"
	case NumberTypeTollFree:
		return "toll-free"
	case NumberTypeShortCode:
		return "short code"
	default:
		return "unknown"
	}
}

// tollFreePrefixes are the US/CA toll-free area codes.
var tollFreePrefixes = []string{"800", "888", "877", "866", "855", "844", "833"}

// NormalizeNumber ensures a phone number has the + prefix for E.164 format.
func NormalizeNumber(number string) string {
	if !strings.HasPrefix(number, "+") {
		return "+" + number
	}
	return number
}

// NormalizeE164 converts common NANP input forms (10-digit, 1-prefixed
// 11-digit, already-E.164, or any of those with spaces/dashes/dots/parens)
// to full E.164 (+1XXXXXXXXXX).
func NormalizeE164(number string) string {
	stripped := strings.NewReplacer(" ", "", "-", "", ".", "", "(", "", ")", "").Replace(number)
	if strings.HasPrefix(stripped, "+") {
		return stripped
	}
	if len(stripped) == 10 {
		return "+1" + stripped
	}
	if len(stripped) == 11 && strings.HasPrefix(stripped, "1") {
		return "+" + stripped
	}
	return "+" + stripped
}

// ClassifyNumber determines the number type from an E.164-formatted phone number.
func ClassifyNumber(number string) NumberType {
	// Strip the + prefix if present
	n := strings.TrimPrefix(number, "+")

	// Short codes are 5-6 digits (no country code prefix)
	if len(n) >= 5 && len(n) <= 6 {
		return NumberTypeShortCode
	}

	// US/CA numbers start with 1 and are 11 digits total
	if len(n) == 11 && strings.HasPrefix(n, "1") {
		areaCode := n[1:4]
		for _, prefix := range tollFreePrefixes {
			if areaCode == prefix {
				return NumberTypeTollFree
			}
		}
		return NumberType10DLC
	}

	return NumberTypeUnknown
}
