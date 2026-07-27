package sip

import (
	"regexp"
	"strings"
	"testing"
)

func TestComputeHashes_KnownVector(t *testing.T) {
	// Recipe validated live: Hash1 = MD5(user:realm:pass),
	// Hash1b = MD5(user@realm:realm:pass). Realm is the full FQDN.
	h1, h1b := ComputeHashes("clitest", "bwclitest-3efeaa.auth.bandwidth.com", "Tb7xQ2mK9rL4vN8s")

	if h1 != "1be6abcaa8e9956021d30f33a3925b99" {
		t.Errorf("Hash1 = %q, want 1be6abcaa8e9956021d30f33a3925b99", h1)
	}
	if h1b != "e028e6577a0bb1b90a33d30a110dbdfe" {
		t.Errorf("Hash1b = %q, want e028e6577a0bb1b90a33d30a110dbdfe", h1b)
	}
}

func TestComputeHashes_LowercaseHex(t *testing.T) {
	h1, h1b := ComputeHashes("u", "r.auth.bandwidth.com", "p")
	for name, h := range map[string]string{"Hash1": h1, "Hash1b": h1b} {
		if len(h) != 32 {
			t.Errorf("%s length = %d, want 32", name, len(h))
		}
		if h != strings.ToLower(h) {
			t.Errorf("%s = %q, want lowercase hex", name, h)
		}
	}
}

func TestGeneratePassword_Shape(t *testing.T) {
	seen := map[string]bool{}
	re := regexp.MustCompile(`^[A-Za-z0-9]{22}$`)
	for i := 0; i < 50; i++ {
		pw, err := GeneratePassword()
		if err != nil {
			t.Fatalf("GeneratePassword() error = %v", err)
		}
		if !re.MatchString(pw) {
			t.Fatalf("password %q does not match ^[A-Za-z0-9]{22}$", pw)
		}
		if seen[pw] {
			t.Fatalf("duplicate password generated: %q", pw)
		}
		seen[pw] = true
	}
}

func TestValidateRealmName(t *testing.T) {
	valid := []string{"a", "vapi", "vapi-test", "a1", "abc123def456ghi789jkl012mno34"}
	for _, n := range valid {
		if err := ValidateRealmName(n); err != nil {
			t.Errorf("ValidateRealmName(%q) = %v, want nil", n, err)
		}
	}
	invalid := []string{"", "-vapi", "vapi-", "va pi", "vapi.test", "VAPI_TEST",
		"abcdefghij0123456789abcdefghij0"} // 31 chars
	for _, n := range invalid {
		if err := ValidateRealmName(n); err == nil {
			t.Errorf("ValidateRealmName(%q) = nil, want error", n)
		}
	}
}

func TestValidateUsername(t *testing.T) {
	if err := ValidateUsername("vapi-agent"); err != nil {
		t.Errorf("ValidateUsername(vapi-agent) = %v, want nil", err)
	}
	// ':' and '@' would corrupt hash construction; empty is meaningless.
	for _, u := range []string{"", "user:name", "user@name"} {
		if err := ValidateUsername(u); err == nil {
			t.Errorf("ValidateUsername(%q) = nil, want error", u)
		}
	}
}
