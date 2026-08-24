package customerprofile

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
)

func TestRoleGateErrorMapsForbiddenToExitConflict(t *testing.T) {
	err := roleGateError(&api.APIError{StatusCode: 403, Body: "does not have access rights"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d (ExitConflict) — a missing role must not read as a reauth prompt", got, cmdutil.ExitConflict)
	}
	msg := err.Error()
	if !strings.Contains(msg, "Customer Profiles Access") {
		t.Errorf("message = %q, want it to name the Customer Profiles Access role", msg)
	}
	if !strings.Contains(msg, "account manager") {
		t.Errorf("message = %q, want it to say an account manager must grant the role", msg)
	}
	if !strings.Contains(msg, "retrying will not help") {
		t.Errorf("message = %q, want it to say retrying will not help", msg)
	}
}

func TestRoleGateErrorPassesThroughNonForbidden(t *testing.T) {
	orig := &api.APIError{StatusCode: 500, Body: "boom"}
	err := roleGateError(orig)
	if err != orig {
		t.Errorf("err = %v, want the original 500 error passed through unchanged", err)
	}
	if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitGeneral {
		t.Errorf("exit code = %d, want %d", got, cmdutil.ExitGeneral)
	}
}

func TestRoleGateErrorPassesThroughNil(t *testing.T) {
	if err := roleGateError(nil); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

func TestRoleGateErrorPassesThroughNonAPIError(t *testing.T) {
	orig := cmdutil.NewFlagError("some flag error")
	if err := roleGateError(orig); err != orig {
		t.Errorf("err = %v, want the non-APIError passed through unchanged", err)
	}
}

// TestServiceForbiddenExitsFour is the end-to-end guard: every command in
// this package must route a 403 from the service through roleGateError, not
// let it fall through to ExitCodeForError's raw 401/403 -> ExitAuth (2)
// mapping. A stray command that forgets this would send an agent chasing
// re-auth for a problem re-auth cannot fix.
func TestServiceForbiddenExitsFour(t *testing.T) {
	forbidden := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"description":"does not have access rights"}]}`))
	}

	cases := []struct {
		name string
		args []string
	}{
		{"create", []string{"create", "--name", "Acme"}},
		{"list", []string{"list"}},
		{"get", []string{"get", "abc"}},
		{"delete", []string{"delete", "abc", "--confirm"}},
		{"history list", []string{"history", "list", "abc"}},
		{"history get", []string{"history", "get", "abc", "2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runCmd(t, forbidden, tc.args...)
			if err == nil {
				t.Fatal("expected an error on a 403 response")
			}
			if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitConflict {
				t.Errorf("exit code = %d, want %d (ExitConflict)", got, cmdutil.ExitConflict)
			}
			if !strings.Contains(err.Error(), "Customer Profiles Access") {
				t.Errorf("error = %q, want an actionable message naming the role", err.Error())
			}
		})
	}
}

// update and restore both GET before they write, so their 403 case is
// exercised separately: the stub must answer the GET with 403 directly
// (there is nothing to overlay yet).
func TestUpdateAndRestoreForbiddenOnGetExitsFour(t *testing.T) {
	forbidden := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"description":"does not have access rights"}]}`))
	}

	for _, args := range [][]string{
		{"update", "abc", "--name", "New"},
		{"restore", "abc"},
	} {
		_, err := runCmd(t, forbidden, args...)
		if err == nil {
			t.Fatalf("%v: expected an error on a 403 response", args)
		}
		if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitConflict {
			t.Errorf("%v: exit code = %d, want %d (ExitConflict)", args, got, cmdutil.ExitConflict)
		}
	}
}

func TestRequireConfirmRejectsWithoutConfirm(t *testing.T) {
	err := requireConfirm(false, "pass --confirm to proceed")
	if err == nil {
		t.Fatal("expected an error when confirm is false")
	}
	if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d (ExitFlagError) — no request should have been made", got, cmdutil.ExitFlagError)
	}
}

func TestRequireConfirmAllowsWithConfirm(t *testing.T) {
	if err := requireConfirm(true, "pass --confirm to proceed"); err != nil {
		t.Errorf("err = %v, want nil when confirm is true", err)
	}
}
