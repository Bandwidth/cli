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

// realmNameRe enforces a DNS label. The two alternatives exist because a single
// pattern permitting a trailing hyphen would emit an invalid hostname label.
var realmNameRe = regexp.MustCompile(`^([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9-]{0,28}[A-Za-z0-9])$`)

// ComputeHashes returns the SIP digest hashes Bandwidth requires when creating
// or rotating a credential. Bandwidth does not compute these server-side.
//
// realmFQDN must be the realm's full hostname exactly as returned by the API
// (e.g. "vapi-3efeaa.auth.bandwidth.com"), not the short realm name — the SIP
// server presents the FQDN in its digest challenge.
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

// ValidateRealmName checks that name is a usable DNS label. The realm name
// becomes the first label of the realm's FQDN.
func ValidateRealmName(name string) error {
	if name == "" {
		return fmt.Errorf("realm name is required")
	}
	if len(name) > 30 {
		return fmt.Errorf("realm name %q is %d characters; maximum is 30", name, len(name))
	}
	if !realmNameRe.MatchString(name) {
		return fmt.Errorf("realm name %q must be alphanumeric with internal hyphens only (a-z, A-Z, 0-9, -)", name)
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
