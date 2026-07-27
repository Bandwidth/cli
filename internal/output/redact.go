package output

import (
	"regexp"
	"strings"
)

// redactedKeys are password-equivalent in SIP digest auth. Matching ignores
// case and any XML namespace prefix.
var redactedKeys = map[string]bool{
	"hash1":  true,
	"hash1b": true,
}

// hashElementRe matches <Hash1>…</Hash1> / <Hash1b>…</Hash1b>, with or without
// a namespace prefix, case-insensitively.
var hashElementRe = regexp.MustCompile(`(?is)<((?:\w+:)?hash1b?)>.*?</(?:\w+:)?hash1b?>`)

func isRedactedKey(k string) bool {
	if i := strings.LastIndex(k, ":"); i >= 0 {
		k = k[i+1:]
	}
	return redactedKeys[strings.ToLower(k)]
}

// RedactSecrets returns a copy of data with digest-hash keys removed at every
// depth. The generated "password" field is deliberately preserved: it is the
// intended output of credential create/rotate.
func RedactSecrets(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for k, val := range v {
			if isRedactedKey(k) {
				continue
			}
			out[k] = RedactSecrets(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, val := range v {
			out[i] = RedactSecrets(val)
		}
		return out
	default:
		return data
	}
}

// RedactAndPrint writes data through RedactSecrets. Commands that can handle
// secret-bearing payloads must print through this rather than StdoutAuto.
func RedactAndPrint(format string, plain bool, data interface{}) error {
	return StdoutAuto(format, plain, RedactSecrets(data))
}

// ScrubHashes removes digest-hash values from a raw XML body while preserving
// the surrounding diagnostic content (error codes, usernames).
func ScrubHashes(s string) string {
	return hashElementRe.ReplaceAllString(s, "<$1>[REDACTED]</$1>")
}
