package customerprofile

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/Bandwidth/cli/internal/api"
	cpsvc "github.com/Bandwidth/cli/internal/customerprofile"
	"github.com/Bandwidth/cli/internal/testutil"
)

// testRoot is a single fake root, built once via testutil.NewTestRoot and
// reused by every runCmd call in this package, with Cmd as its only child.
// --format/--plain/--account-id/--environment live here, not on Cmd, exactly
// as in production (Cmd carries none of its own — see customerprofile.go).
//
// It is deliberately NOT rebuilt per call. cobra caches each command's merged
// ancestor flags (parentsPflags) the first time it parses and never
// refreshes that cache for a different root object later: constructing a
// fresh testutil.NewTestRoot(Cmd) inside runCmd on every invocation would
// mean Cmd/createCmd/listCmd/getCmd — all package-level, so the same
// instances across every test — get pinned to the FIRST test's root the
// first time any of them parses, and every later --plain/--format/
// --account-id would silently parse into that stale, discarded root's flag
// object while cmd.Root().Flag(...) reads the current (untouched, always
// default) root. Verified with a minimal cobra repro before writing this:
// a fresh root per call reports --plain as false on every call after the
// first; a single shared root with its own flags reset between calls
// reports it correctly every time. See task-4-report.md, "Fix round 1".
var testRoot = testutil.NewTestRoot(Cmd)

// resetFlags restores every flag on cmd and all its descendants to its
// default value and clears the Changed bit.
//
// Cmd is a package-level cobra command, and cobra records flag state
// (including Changed) on it permanently. Within one test binary, every
// Cmd.Execute() call in this package shares that state: a flag set by an
// earlier test (e.g. --name from a create test) would otherwise leak into a
// later run, and cmd.Flags().Changed("offset") would stay true for the rest
// of the process once any test passes --offset — silently breaking
// TestListRejectsAllWithExplicitOffset or making it pass for the wrong
// reason. resetFlags is walked recursively (rather than listing flag names
// per command) so it does not need updating as later tasks add more
// commands and flags to this tree. It is called on testRoot, not Cmd, so it
// also resets testRoot's own --format/--plain/--account-id/--environment
// (recursion reaches Cmd and its subcommands via testRoot.Commands()).
func resetFlags(cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
	cmd.Flags().VisitAll(reset)
	cmd.PersistentFlags().VisitAll(reset)
	for _, sub := range cmd.Commands() {
		resetFlags(sub)
	}
}

func TestCommandsRegistered(t *testing.T) {
	for _, name := range []string{"create", "list", "get"} {
		c, _, err := Cmd.Find([]string{name})
		if err != nil || c.Name() != name {
			t.Errorf("Find(%q) = %v, err %v", name, c, err)
		}
	}
}

func TestCreateRequiresName(t *testing.T) {
	out, err := runCmd(t, nil, "create")
	if err == nil {
		t.Fatal("expected an error when --name is missing")
	}
	if !strings.Contains(err.Error(), "missing required flags") ||
		!strings.Contains(err.Error(), "--name") {
		t.Errorf("error = %q, want it to name the missing flag", err.Error())
	}
	if out != "" {
		t.Errorf("stdout = %q, want nothing written on a flag error", out)
	}
}

func TestCreateEmitsReceipt(t *testing.T) {
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"abc","name":"Acme","version":0}}`))
	}, "create", "--name", "Acme", "--plain")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not JSON: %q", out)
	}
	if got["id"] != "abc" {
		t.Errorf("stdout id = %v, want abc", got["id"])
	}
}

func TestListReturnsArrayEvenForOneResult(t *testing.T) {
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"abc"}],"page":{"pageSize":50,"totalElements":1}}`))
	}, "list", "--plain")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("stdout = %q, want a JSON array", out)
	}
}

func TestListAllWalksEveryPage(t *testing.T) {
	var offsets []string
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		offsets = append(offsets, r.URL.Query().Get("offset"))
		if r.URL.Query().Get("offset") == "" || r.URL.Query().Get("offset") == "0" {
			_, _ = w.Write([]byte(`{"data":[{"id":"a"}],"page":{"pageSize":1,"totalElements":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"b"}],"page":{"pageSize":1,"totalElements":2}}`))
	}, "list", "--all", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("list --all: %v", err)
	}
	if !strings.Contains(out, `"a"`) || !strings.Contains(out, `"b"`) {
		t.Errorf("stdout = %q, want items from both pages", out)
	}
}

func TestListRejectsAllWithExplicitOffset(t *testing.T) {
	_, err := runCmd(t, nil, "list", "--all", "--offset", "0")
	if err == nil {
		t.Fatal("expected an error: --all with an explicit --offset is contradictory")
	}
}

func TestGetReturnsObjectNotArray(t *testing.T) {
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"abc","softDeleted":false}}`))
	}, "get", "abc", "--plain")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("stdout = %q, want a JSON object", out)
	}
}

func TestGetRequiresID(t *testing.T) {
	if _, err := runCmd(t, nil, "get"); err == nil {
		t.Fatal("expected an error when no ID is given")
	}
}

func TestFilterFlagUsesDeepObjectEncoding(t *testing.T) {
	var rawQuery string
	_, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[],"page":{"pageSize":50,"totalElements":0}}`))
	}, "list", "--name-contains", "Acme", "--plain")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(rawQuery, "name%5Bcontains%5D=Acme") {
		t.Errorf("query = %q, want deepObject form name[contains]=Acme", rawQuery)
	}
}

// runCmd executes one command against a stub server and returns stdout.
// Every command test in this package goes through it, so the seam is
// swapped in exactly one place.
func runCmd(t *testing.T, h http.HandlerFunc, args ...string) (string, error) {
	t.Helper()

	resetFlags(testRoot)

	var srvURL string
	if h != nil {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		srvURL = srv.URL
	}

	orig := service
	service = func(cmd *cobra.Command) (*cpsvc.Service, error) {
		if srvURL == "" {
			t.Fatal("command made a request but no stub server was provided")
		}
		return cpsvc.NewService(api.NewClientNoAuth(srvURL), "9901287"), nil
	}
	t.Cleanup(func() { service = orig })

	testRoot.SetArgs(append([]string{Cmd.Name()}, args...))
	testRoot.SetOut(io.Discard)
	testRoot.SetErr(io.Discard)

	var err error
	out := testutil.CaptureStdout(t, func() { err = testRoot.Execute() })
	return out, err
}
