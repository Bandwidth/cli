package tendlc

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
)

func TestRoleGateError_RegistrationCenter(t *testing.T) {
	err := roleGateError(&api.APIError{
		StatusCode: 403,
		Body:       `{"errors":[{"type":"forbidden","description":"Account 33333 is not enabled for the Registration Center"}]}`,
	}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "not enabled for the Registration Center") {
		t.Errorf("got %q, want Registration Center message", got)
	}
}

func TestRoleGateError_ImportCustomer(t *testing.T) {
	err := roleGateError(&api.APIError{
		StatusCode: 403,
		Body:       `{"errors":[{"type":"forbidden","description":"'10DLC campaign management' import customer is not enabled on account 33333"}]}`,
	}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "register campaigns through TCR") {
		t.Errorf("got %q, want import customer message", got)
	}
}

func TestRoleGateError_FeatureNotEnabled(t *testing.T) {
	err := roleGateError(&api.APIError{
		StatusCode: 403,
		Body:       `{"errors":[{"type":"forbidden","description":"'10DLC campaign management' is not enabled on account 33333"}]}`,
	}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "campaign management is not enabled") {
		t.Errorf("got %q, want feature not enabled message", got)
	}
}

func TestRoleGateError_NoRole(t *testing.T) {
	err := roleGateError(&api.APIError{
		StatusCode: 403,
		Body:       `{"errors":[{"type":"forbidden","description":"client does not have access rights to the content"}]}`,
	}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "Campaign Management role") {
		t.Errorf("got %q, want role message", got)
	}
}

func TestRoleGateError_UnknownBody(t *testing.T) {
	err := roleGateError(&api.APIError{StatusCode: 403, Body: "something unexpected"}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "access denied (403)") {
		t.Errorf("got %q, want fallback message", got)
	}
}

func TestRoleGateError_OtherStatus(t *testing.T) {
	err := roleGateError(&api.APIError{StatusCode: 500, Body: "Internal Server Error"}, "Campaign Management")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); !contains(got, "API request failed") {
		t.Errorf("got %q, want API request failed", got)
	}
}

func TestRoleGateError_NonAPIError(t *testing.T) {
	err := roleGateError(&api.APIError{StatusCode: 404, Body: "Not Found"}, "TFV")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStatusCommandRegistered(t *testing.T) {
	c, _, err := Cmd.Find([]string{"status"})
	if err != nil || c.Name() != "status" {
		t.Fatalf("Find(status) = %v, err %v; want the status command", c, err)
	}
}

func TestStatusResultShape(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantAccess string
		wantReason string
	}{
		{"success", 200, `{"data":[]}`, "available", "probe_succeeded"},
		{"role missing", 403, `{"errors":[{"description":"does not have access rights"}]}`,
			"unavailable", "role_absent"},
		{"registration center off", 403, `{"errors":[{"description":"not enabled for the Registration Center"}]}`,
			"unavailable", "registration_center_not_enabled"},
		{"campaign mgmt off", 403, `{"errors":[{"description":"10DLC is not enabled on account"}]}`,
			"unavailable", "campaign_management_not_enabled"},
		{"unrecognized 403", 403, `{"errors":[{"description":"something new"}]}`,
			"unavailable", "access_denied"},
		{"server error", 503, `{}`, "unknown", "probe_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusResult(tt.statusCode, tt.body)
			if got["access"] != tt.wantAccess {
				t.Errorf("access = %v, want %v", got["access"], tt.wantAccess)
			}
			if got["reason"] != tt.wantReason {
				t.Errorf("reason = %v, want %v", got["reason"], tt.wantReason)
			}
		})
	}
}

// mode is always present and always unknown — an omitted field invites
// callers to guess a default.
func TestStatusAlwaysReportsModeUnknown(t *testing.T) {
	for _, code := range []int{200, 403, 503} {
		got := statusResult(code, `{}`)
		mode, ok := got["mode"].(map[string]string)
		if !ok {
			t.Fatalf("code %d: mode missing or wrong type: %#v", code, got["mode"])
		}
		if mode["status"] != "unknown" || mode["reason"] != "not_discoverable" {
			t.Errorf("code %d: mode = %v, want unknown/not_discoverable", code, mode)
		}
	}
}

// isNotFound is the sole translator of a real 404 into pollTarget's
// found=false, which is the mechanism the create-vs-delete GoneIsDone
// contract in async.go depends on — it must recognize a 404 wherever it
// appears in the error chain, and reject everything else, including a nil
// error.
func TestIsNotFound(t *testing.T) {
	notFound := &api.APIError{StatusCode: 404, Body: "brand not found"}
	serverError := &api.APIError{StatusCode: 500, Body: "boom"}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"404 API error", notFound, true},
		{"500 API error", serverError, false},
		{"wrapped 404", fmt.Errorf("fetching brand: %w", notFound), true},
		{"plain non-API error", errors.New("connection reset"), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNotFound(tt.err); got != tt.want {
				t.Errorf("isNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestRemovedLegacyCommandsExitNonZero locks in the Task 3 fix: a stray
// token that used to be a real command -- `campaigns`, `numbers`, or a bare
// positional under `number` -- must fail loudly, not print help with exit 0.
// Before this fix, Cmd/brandCmd/campaignCmd/numberCmd/vettingCmd had no RunE,
// so cobra's execute() hit its `if !c.Runnable() { return flag.ErrHelp }`
// short-circuit before ever reaching ValidateArgs, and a deleted command's
// name looked identical to a successful help request: exit 0 either way. A
// caller (especially an agent scripting against this CLI) cannot tell
// "this command doesn't exist" apart from "you asked for help" on exit code
// alone without this fix.
//
// `number +15555550100` is the subtler case of the three: "+15555550100" was
// never a command name to begin with, so this isn't "unknown command", it's
// a stray positional against a parent (numberCmd) that now takes none. Both
// failure shapes are covered here because they go through different cobra
// code paths (unmatched child name vs. a leftover arg after the deepest
// match), and only NoArgs on the matched command catches the latter.
//
// No stub server is passed (srv is nil in every case): all three must fail
// before ever reaching a RunE that would call `service`, so this needs no
// live API call and no credentials.
//
// {"brand", "STRAY"} and {"vetting", "STRAY"} extend this to the other two
// parents named in the doc comment above (Cmd itself is exercised by
// "campaigns"/"numbers"; numberCmd by "number +15555550100"; campaignCmd by
// "campaign STRAY" below) -- without this, brandCmd or vettingCmd losing its
// RunE would silently regress to exit 0 on a stray positional with nothing
// in this suite catching it.
func TestRemovedLegacyCommandsExitNonZero(t *testing.T) {
	cases := [][]string{
		{"campaigns"},
		{"numbers"},
		{"number", "+15555550100"},
		{"brand", "STRAY"},
		{"campaign", "STRAY"},
		{"vetting", "STRAY"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, err := runBrandCmd(t, nil, args...)
			if err == nil {
				t.Fatalf("band tendlc %s: want a non-zero-exit error, got nil", strings.Join(args, " "))
			}
			if code := cmdutil.ExitCodeForError(err); code == cmdutil.ExitOK {
				t.Errorf("band tendlc %s: exit code = %d, want non-zero", strings.Join(args, " "), code)
			}
		})
	}
}

// TestParentCommandsStillDispatchToRealSubcommands guards the other half of
// the same change: Args: cobra.NoArgs on Cmd/brandCmd/campaignCmd/numberCmd/
// vettingCmd must only reject a token that matches no subcommand -- cobra
// resolves subcommands via Find before Args is ever consulted, so a real
// subcommand name must keep dispatching exactly as before. Checked directly
// via Cmd.Find rather than execution, since these commands need no stub
// server or credentials to prove routing.
func TestParentCommandsStillDispatchToRealSubcommands(t *testing.T) {
	cases := []struct {
		path []string
		want *cobra.Command
	}{
		{[]string{"number", "list"}, numberListCmd},
		{[]string{"brand", "list"}, brandListCmd},
		{[]string{"campaign", "list"}, campaignListCmd},
		{[]string{"vetting", "list"}, vettingListCmd},
	}
	for _, tt := range cases {
		name := strings.Join(tt.path, " ")
		t.Run(name, func(t *testing.T) {
			found, _, err := Cmd.Find(tt.path)
			if err != nil {
				t.Fatalf("Find(%v): %v", tt.path, err)
			}
			if found != tt.want {
				t.Errorf("%s resolved to %q, want %s", name, found.CommandPath(), tt.want.Name())
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
