// Package sip provides domain logic for Bandwidth SIP realm and credential
// provisioning: digest-hash computation, password generation, and validation.
package sip

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// passwordAlphabet is intentionally alphanumeric-only: SIP configuration files
// and shell round-trips mangle punctuation, and 62^22 still yields ~131 bits.
const passwordAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

const passwordLength = 22

// realmNameRe enforces the Bandwidth API constraint for realm names (error 33013):
// lowercase alphanumeric only. This is stricter than a DNS label — hyphens and
// uppercase letters are rejected by the API even though DNS labels permit them.
var realmNameRe = regexp.MustCompile(`^[a-z0-9]+$`)

// ComputeHashes returns the SIP digest hashes Bandwidth requires when creating
// or rotating a credential. Bandwidth does not compute these server-side.
//
// realmFQDN must be the realm's full hostname exactly as returned by the API
// (e.g. "vapi-3efeaa.auth.bandwidth.com"), not the short realm name — the SIP
// server presents the FQDN in its digest challenge.
//
// On the use of MD5: this is not a security choice available to us. SIP/HTTP
// digest authentication (RFC 2617) defines HA1 as MD5(user:realm:pass), and the
// Bandwidth API accepts only MD5 digests for Hash1/Hash1b — it stores what we
// send, and its SIP registrar validates against an MD5 challenge. Substituting a
// stronger hash would not harden anything; it would produce a credential that no
// SIP peer could ever authenticate with. Static analysers flag this line as
// CWE-327 ("Use of a Broken or Risky Cryptographic Algorithm"); the finding is
// understood and cannot be remediated without breaking protocol interoperability.
//
// The property that does matter is that the OUTPUT is password-equivalent:
// anyone holding Hash1 can authenticate as this credential. That is why the
// domain types carry no hash fields, why output is redacted, and why error
// bodies are scrubbed before being stored on an error.
func ComputeHashes(username, realmFQDN, password string) (hash1 string, hash1b string) {
	sum := func(s string) string {
		h := md5.Sum([]byte(s))
		return hex.EncodeToString(h[:])
	}
	hash1 = sum(username + ":" + realmFQDN + ":" + password)
	hash1b = sum(username + "@" + realmFQDN + ":" + realmFQDN + ":" + password)
	return hash1, hash1b
}

// GeneratePassword returns a cryptographically random alphanumeric password.
func GeneratePassword() (string, error) {
	max := big.NewInt(int64(len(passwordAlphabet)))
	var b strings.Builder
	b.Grow(passwordLength)
	for i := 0; i < passwordLength; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generating password: %w", err)
		}
		b.WriteByte(passwordAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// ValidateRealmName checks that name satisfies the Bandwidth API constraint.
// The name becomes the first label of the realm's FQDN.
func ValidateRealmName(name string) error {
	if name == "" {
		return fmt.Errorf("realm name is required")
	}
	if len(name) > 30 {
		return fmt.Errorf("realm name %q is %d characters; maximum is 30", name, len(name))
	}
	if !realmNameRe.MatchString(name) {
		return fmt.Errorf("realm name %q must be lowercase alphanumeric only (a-z, 0-9)", name)
	}
	return nil
}

// appIDRe matches the canonical 8-4-4-4-12 hex UUID form. The check is
// deliberately strict rather than a "looks like an ID" heuristic: the API pins
// HttpVoiceV2AppId to a UUID, and the alternative — letting a typo or a
// documentation placeholder through — costs a realm lookup, a password
// generation, and (with --generate-password) a trip down the write-once path
// before the live API rejects it.
var appIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidateAppID checks that a voice application ID is a UUID. An empty value is
// valid and means "unbound" — absent and "" are the same state to the API.
func ValidateAppID(appID string) error {
	if appID == "" {
		return nil
	}
	if !appIDRe.MatchString(appID) {
		return fmt.Errorf("--app-id %q must be a UUID (e.g. 04e88489-df02-4e34-a0e2-4d0e0d3f7a1c); find yours with 'band app list --plain'", appID)
	}
	return nil
}

// ValidateUsername rejects usernames that would corrupt digest-hash construction.
func ValidateUsername(username string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if strings.ContainsAny(username, ":@") {
		return fmt.Errorf("username %q must not contain ':' or '@'", username)
	}
	return nil
}
