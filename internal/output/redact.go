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
// a namespace prefix and optional attributes, case-insensitively.
var hashElementRe = regexp.MustCompile(`(?is)<((?:\w+:)?hash1b?)(?:\s[^>]*)?>.*?</(?:\w+:)?hash1b?>`)

// bareHashRe matches a bare 32-lowercase-hex token: the rendered form of an MD5
// digest (ComputeHashes emits lowercase hex). The element-anchored regex above
// cannot catch a hash echoed in prose — the live 23026 response echoes submitted
// hashes inside the error Description, e.g. "Invalid Hash1 value d41d8cd9…" —
// nor one truncated before its closing tag.
//
// This is applied ONLY by ScrubHashes, i.e. only to raw XML response/error
// bodies and fault descriptions. It is deliberately NOT used by RedactSecrets:
// a 32-hex value sitting in a structured output field is far more likely to be a
// legitimate identifier than a digest, and dropping the length/case constraint
// there would risk mangling real data.
var bareHashRe = regexp.MustCompile(`\b[0-9a-f]{32}\b`)

func isRedactedKey(k string) bool {
	if i := strings.LastIndex(k, ":"); i >= 0 {
		k = k[i+1:]
	}
	return redactedKeys[strings.ToLower(k)]
}

// RedactSecrets returns a copy of data with digest-hash keys removed at every
// depth within maps and slices. The generated "password" field is deliberately
// preserved: it is the intended output of credential create/rotate.
//
// NOTE: RedactSecrets only redacts within map[string]interface{} and
// []interface{} structures. It does not scrub digest-hash fields from typed
// structs: a struct field named Hash1 or Hash1b will pass through unchanged.
// Callers must ensure that struct types carrying hash fields are not routed
// through this function. (The typical path decodes API responses to
// map[string]interface{} via XMLToMap, and domain structs deliberately omit
// hash fields, so this is safe in practice.)
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

// ScrubHashes removes digest-hash values from a raw XML body or error
// description while preserving the surrounding diagnostic content (error codes,
// usernames). It runs two passes: the element-anchored one, which keeps the
// XML shape intact, then a bare 32-hex sweep for hashes echoed as prose or left
// unterminated by a truncated body.
func ScrubHashes(s string) string {
	s = hashElementRe.ReplaceAllString(s, "<$1>[REDACTED]</$1>")
	return bareHashRe.ReplaceAllString(s, "[REDACTED]")
}
