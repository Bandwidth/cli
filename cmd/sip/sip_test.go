package sip

import (
	"errors"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
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

// TestFaultExit_PreservesExitCode guards against a regression where a
// documented error code's branch in faultExit wraps the original *APIFault in
// a plain fmt.Errorf (no %w), silently detaching it from errors.As. Because
// cmdutil.ExitCodeForError determines the process exit code purely by
// unwrapping to *FeatureLimitError / *api.APIError / ErrPollTimeout, a
// dropped wrap makes a documented conflict (should exit 4) fall through to
// exit 1 — indistinguishable from an unexpected failure to an agent branching
// on exit codes.
func TestFaultExit_PreservesExitCode(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{"default realm delete conflict", "33006"},
		{"realm has credentials conflict", "12666"},
		{"realm not active yet conflict", "23022"},
		{"realm already exists conflict", "33002"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fault := &sipsvc.APIFault{Code: c.code, Description: "boom", StatusCode: 409}
			result := faultExit(fault)
			if got := cmdutil.ExitCodeForError(result); got != cmdutil.ExitConflict {
				t.Errorf("faultExit(%s) -> ExitCodeForError = %d, want ExitConflict (%d): err = %v",
					c.code, got, cmdutil.ExitConflict, result)
			}
		})
	}

	// 33004 must still map to ExitConflict via FeatureLimitError (regression
	// guard for the one branch that was already correct).
	fault := &sipsvc.APIFault{Code: "33004", Description: "not enabled", StatusCode: 403}
	result := faultExit(fault)
	if got := cmdutil.ExitCodeForError(result); got != cmdutil.ExitConflict {
		t.Errorf("faultExit(33004) -> ExitCodeForError = %d, want ExitConflict (%d): err = %v",
			got, cmdutil.ExitConflict, result)
	}
}

// TestFaultExit_DuplicateCredentialPreservesAPIFaultWrapping guards the same
// %w regression as TestFaultExit_PreservesExitCode for the 23026 branch. It
// does not assert a specific numeric exit code: CreateCredential's live
// duplicate-credential fault has been observed at both StatusCode 201 (2xx
// body carrying an Errors envelope) and, per code review, potentially 400 —
// neither of which ExitCodeForError's api.APIError switch maps to
// ExitConflict (it only maps 402/409). The 23026 branch here improves the
// remediation message; it does not change the resulting exit code, and this
// test only confirms the branch does not silently drop the *APIFault via a
// bare fmt.Errorf.
func TestFaultExit_DuplicateCredentialPreservesAPIFaultWrapping(t *testing.T) {
	fault := &sipsvc.APIFault{Code: "23026", Description: "does already exist", StatusCode: 201}
	result := faultExit(fault)
	var got *sipsvc.APIFault
	if !errors.As(result, &got) {
		t.Fatalf("faultExit(23026) = %v, want an error that unwraps to *sipsvc.APIFault", result)
	}
}
