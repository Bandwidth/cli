package sip

import (
	"testing"

	sipsvc "github.com/Bandwidth/cli/internal/sip"
)

func TestRealmReuseAllowed(t *testing.T) {
	cases := []struct {
		status string
		wantOK bool
	}{
		{"ACTIVE", true},
		{"CREATE_PENDING", true},
		{"CREATE_FAILED", false},
		{"DELETE_PENDING", false},
		{"DELETE_FAILED", false},
		{"SOMETHING_NEW", false},
	}
	for _, c := range cases {
		if got := realmReuseAllowed(c.status); got != c.wantOK {
			t.Errorf("realmReuseAllowed(%q) = %v, want %v", c.status, got, c.wantOK)
		}
	}
}

func TestRealmStateMatches(t *testing.T) {
	existing := &sipsvc.Realm{Name: "vapi", Default: false, Description: "d"}

	// Only fields the caller specified participate in the comparison.
	if !realmStateMatches(existing, false, "d", true) {
		t.Error("identical state reported as mismatch")
	}
	if !realmStateMatches(existing, false, "", false) {
		t.Error("unspecified description must not cause a mismatch")
	}
	if realmStateMatches(existing, true, "", false) {
		t.Error("differing Default must be reported as a mismatch")
	}
	if realmStateMatches(existing, false, "other", true) {
		t.Error("differing description must be reported as a mismatch")
	}
}
