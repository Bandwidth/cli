package sip

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	sipsvc "github.com/Bandwidth/cli/internal/sip"
	"github.com/Bandwidth/cli/internal/testutil"
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

// TestFaultExit_PreservesExitCode asserts that every documented conflict maps
// to exit 4 regardless of the HTTP status the API used to report it.
//
// The StatusCode in each fixture is the LIVE-VERIFIED one, which is the whole
// point: an earlier version of this test built 23022 with StatusCode 409 — a
// status that endpoint never returns — so it passed while the real 400 fell
// through to exit 1. Exit codes must come from the error type, not the status.
func TestFaultExit_PreservesExitCode(t *testing.T) {
	cases := []struct {
		name   string
		code   string
		status int
	}{
		{"default realm delete conflict", "33006", 409},
		{"realm has credentials conflict", "12666", 409},
		{"realm already exists conflict", "33002", 409},
		// Live: 23022 is a 400, not a 409.
		{"realm not active yet conflict", "23022", 400},
		// Live: 23026 arrives as a 400, and as a 201 carrying an Errors
		// envelope on bulk create. Neither status maps to 4 on its own.
		{"duplicate credential conflict (400)", "23026", 400},
		{"duplicate credential conflict (201 partial success)", "23026", 201},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fault := &sipsvc.APIFault{Code: c.code, Description: "boom", StatusCode: c.status}
			result := faultExit(fault)
			if got := cmdutil.ExitCodeForError(result); got != cmdutil.ExitConflict {
				t.Errorf("faultExit(%s @ %d) -> ExitCodeForError = %d, want ExitConflict (%d): err = %v",
					c.code, c.status, got, cmdutil.ExitConflict, result)
			}
			// The original fault must stay reachable so callers can still read
			// the error code behind the conflict.
			var unwrapped *sipsvc.APIFault
			if !errors.As(result, &unwrapped) {
				t.Errorf("faultExit(%s) = %v, want an error that unwraps to *sipsvc.APIFault", c.code, result)
			}
		})
	}

	// 33004 must still map to ExitConflict via FeatureLimitError (regression
	// guard for the one branch that was already correct).
	fault := &sipsvc.APIFault{Code: "33004", Description: "not enabled", StatusCode: 400}
	result := faultExit(fault)
	if got := cmdutil.ExitCodeForError(result); got != cmdutil.ExitConflict {
		t.Errorf("faultExit(33004) -> ExitCodeForError = %d, want ExitConflict (%d): err = %v",
			got, cmdutil.ExitConflict, result)
	}
}

// TestFaultExit_UndocumentedFaultKeepsStatusMapping is the negative pairing:
// faultExit must NOT blanket-convert every fault into a conflict. An
// undocumented code passes through so its status still drives the exit code —
// a 429 must stay retryable (7), not become "stop, conflict" (4).
func TestFaultExit_UndocumentedFaultKeepsStatusMapping(t *testing.T) {
	fault := &sipsvc.APIFault{Code: "1001", Description: "slow down", StatusCode: 429}
	if got := cmdutil.ExitCodeForError(faultExit(fault)); got != cmdutil.ExitRateLimit {
		t.Errorf("ExitCodeForError = %d, want ExitRateLimit (%d)", got, cmdutil.ExitRateLimit)
	}
}

// TestDomainStructsCarryNoHashFields converts emit's structural hash-safety
// assumption into an enforced one. output.RedactSecrets is a net, but the
// primary guarantee is that Realm and Credential simply have nowhere to put a
// digest hash. A future field named Hash1/Hash1b would silently publish
// password-equivalent material through every `band sip` output path.
func TestDomainStructsCarryNoHashFields(t *testing.T) {
	hashFieldRe := regexp.MustCompile(`(?i)^hash1b?$`)
	for _, v := range []interface{}{sipsvc.Realm{}, sipsvc.Credential{}} {
		typ := reflect.TypeOf(v)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if !f.IsExported() {
				continue
			}
			if hashFieldRe.MatchString(f.Name) {
				t.Errorf("%s.%s is a digest-hash field on an output struct — remove it or keep hashes on the wire types only",
					typ.Name(), f.Name)
			}
			if tag := strings.Split(f.Tag.Get("json"), ",")[0]; hashFieldRe.MatchString(tag) {
				t.Errorf("%s.%s serializes as %q, a digest-hash key", typ.Name(), f.Name, tag)
			}
		}
	}
}

// TestEmit_TableRendersFieldsNotGoStructDump guards the --format table path.
// output.printTable has no case for typed structs and falls through to
// fmt.Fprintf("%v"), which printed `&{1103 vapi vapi-3efeaa… false ACTIVE 0}`
// — unlabeled, unparseable, and silently bypassing RedactSecrets. emit
// normalizes to generic JSON values first, so the table renders real columns.
func TestEmit_TableRendersFieldsNotGoStructDump(t *testing.T) {
	realm := &sipsvc.Realm{
		ID: "1103", Name: "vapi", Hostname: "vapi-3efeaa.auth.bandwidth.com",
		Status: "ACTIVE", CredentialCount: 0,
	}
	wrap := &cobra.Command{
		Use: "wrap",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, plain := cmdutil.OutputFlags(cmd)
			return emit(format, plain, realm)
		},
	}
	root := testutil.NewTestRoot(wrap)
	root.SetArgs([]string{"wrap", "--format", "table"})

	out := testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if strings.Contains(out, "&{") {
		t.Errorf("table output is a Go struct dump: %q", out)
	}
	for _, want := range []string{"hostname", "vapi-3efeaa.auth.bandwidth.com", "status", "ACTIVE"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q: %q", want, out)
		}
	}
}

// TestEmit_RedactsHashKeysAtRuntime proves the normalization made redaction
// live rather than structurally inert: a raw map carrying a hash key must lose
// it on the way out, in table format as well as --plain.
func TestEmit_RedactsHashKeysAtRuntime(t *testing.T) {
	payload := map[string]interface{}{
		"id":    "870874",
		"Hash1": "1be6abcaa8e9956021d30f33a3925b99",
	}
	wrap := &cobra.Command{
		Use: "wrap",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, plain := cmdutil.OutputFlags(cmd)
			return emit(format, plain, payload)
		},
	}
	root := testutil.NewTestRoot(wrap)
	root.SetArgs([]string{"wrap", "--format", "table"})

	out := testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})
	if strings.Contains(out, "1be6abcaa8e9956021d30f33a3925b99") || strings.Contains(strings.ToLower(out), "hash1") {
		t.Errorf("hash reached table output: %q", out)
	}
	if !strings.Contains(out, "870874") {
		t.Errorf("non-secret field was lost: %q", out)
	}
}
