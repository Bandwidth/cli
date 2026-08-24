package tendlc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// stubBrandIdentityServer answers any request with a 204 (matching the real
// API's empty-body response for both /identity/reverify and
// /identity/resend2faEmail) and records the path of every request it sees.
func stubBrandIdentityServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var paths []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	})
	return srv, &paths
}

// Test: reverify without --confirm exits 6 and makes ZERO HTTP requests —
// runBrandCmd's `service` seam Fatals if it is ever invoked with a nil
// server, so this fails loudly if the confirm gate ever moves after the
// service/POST call.
func TestBrandReverifyWithoutConfirmMakesNoRequests(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "brand", "reverify", "BGJR2BA")
	if err == nil {
		t.Fatal("want an error when --confirm is missing")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "$4") {
		t.Errorf("error = %q, want it to name the $4 fee", err.Error())
	}
}

// TestBrandReverifyWithConfirmPostsToIdentityReverify locks in the exact
// path so a copy-paste between reverify and resend-2fa (they differ only in
// the URL suffix) cannot go unnoticed.
func TestBrandReverifyWithConfirmPostsToIdentityReverify(t *testing.T) {
	srv, paths := stubBrandIdentityServer(t)

	out, _, err := runBrandCmd(t, srv, "brand", "reverify", "BGJR2BA", "--confirm", "--plain")
	if err != nil {
		t.Fatalf("brand reverify --confirm: %v", err)
	}
	if len(*paths) != 1 || !strings.HasSuffix((*paths)[0], "/brands/BGJR2BA/identity/reverify") {
		t.Fatalf("paths = %v, want exactly one POST to .../identity/reverify", *paths)
	}
	got := decodeStdout(t, out)
	if got["id"] != "BGJR2BA" {
		t.Errorf("stdout = %v, want id BGJR2BA", got)
	}
	if got["reverificationRequested"] != true {
		t.Errorf("stdout = %v, want reverificationRequested true", got)
	}
	if got["status"] != "accepted" {
		t.Errorf("stdout = %v, want status accepted", got)
	}
	if got["check"] != "band tendlc brand get BGJR2BA" {
		t.Errorf("stdout = %v, want check pointing at brand get", got)
	}
}

// TestBrandResend2FANeedsNoConfirm asserts the opposite gate from reverify:
// re-sending an email is free and non-destructive, so no --confirm flag
// should be required, and the command should not even define one.
func TestBrandResend2FANeedsNoConfirm(t *testing.T) {
	srv, paths := stubBrandIdentityServer(t)

	out, _, err := runBrandCmd(t, srv, "brand", "resend-2fa", "BGJR2BA", "--plain")
	if err != nil {
		t.Fatalf("brand resend-2fa: %v", err)
	}
	if len(*paths) != 1 || !strings.HasSuffix((*paths)[0], "/brands/BGJR2BA/identity/resend2faEmail") {
		t.Fatalf("paths = %v, want exactly one POST to .../identity/resend2faEmail", *paths)
	}
	got := decodeStdout(t, out)
	if got["id"] != "BGJR2BA" {
		t.Errorf("stdout = %v, want id BGJR2BA", got)
	}
	if got["emailResent"] != true {
		t.Errorf("stdout = %v, want emailResent true", got)
	}
}

// TestBrandIdentityCommandsRejectStrayPositionals mirrors
// TestBrandCommandsRejectStrayPositionals: both reverify and resend-2fa take
// exactly one positional, so a missing or extra one must be rejected before
// any request is made.
func TestBrandIdentityCommandsRejectStrayPositionals(t *testing.T) {
	cases := [][]string{
		{"brand", "reverify"},
		{"brand", "reverify", "B1", "STRAY", "--confirm"},
		{"brand", "resend-2fa"},
		{"brand", "resend-2fa", "B1", "STRAY"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, _, err := runBrandCmd(t, nil, args...); err == nil {
				t.Fatal("want an argument error")
			}
		})
	}
}

// TestBrandIdentity403MapsToExitFour exercises roleGateError on both
// commands: a 403 from either endpoint must map to exit 4, not the generic
// exit 1 a bare error would produce.
func TestBrandIdentity403MapsToExitFour(t *testing.T) {
	body := `{"errors":[{"description":"does not have access rights"}]}`

	cases := []struct {
		name string
		args []string
	}{
		{"reverify", []string{"brand", "reverify", "BGJR2BA", "--confirm"}},
		{"resend-2fa", []string{"brand", "resend-2fa", "BGJR2BA"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runBrandCmd(t, stubBrandErr(t, 403, body), tt.args...)
			if err == nil {
				t.Fatal("want an error on 403")
			}
			if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
				t.Errorf("exit code = %d, want %d", code, cmdutil.ExitConflict)
			}
		})
	}
}
