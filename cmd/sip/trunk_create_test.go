package sip

import (
	"errors"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/testutil"
)

// swapTrunkClient substitutes trunkClient with fake, restoring it on cleanup; no t.Parallel() on callers (mutates a global).
func swapTrunkClient(t *testing.T, fake *testutil.FakeClient) {
	t.Helper()
	orig := trunkClient
	t.Cleanup(func() { trunkClient = orig })
	trunkClient = func(string) (api.Requester, string, error) {
		return fake, "9900001", nil
	}
	t.Cleanup(func() { trunkTransport = "" })
}

func runTrunkCreateCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := testutil.NewTestRoot(trunkCreateCmd)
	root.SetArgs(append([]string{"create"}, args...))
	var execErr error
	out := testutil.CaptureStdout(t, func() { execErr = root.Execute() })
	return out, execErr
}

func TestSipTrunkPlainOutput(t *testing.T) {
	fake := &testutil.FakeClient{PostResult: map[string]interface{}{
		"links": []interface{}{},
		"data": map[string]interface{}{
			"accountId": "9900001",
			"transport": "TLS",
			"state":     "PROVISIONING",
			"message":   "SIP trunk provisioning started. The trunks are ready once it completes.",
		},
		"errors": []interface{}{},
	}}
	swapTrunkClient(t, fake)

	out, err := runTrunkCreateCmd(t, "--transport", "tls", "--plain")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	want := "{\n  \"accountId\": \"9900001\",\n  \"message\": \"SIP trunk provisioning started. The trunks are ready once it completes.\",\n  \"state\": \"PROVISIONING\",\n  \"transport\": \"TLS\"\n}\n"
	if out != want {
		t.Fatalf("golden mismatch:\n got: %q\nwant: %q", out, want)
	}
	if fake.PostPath != "/accounts/9900001/sip-trunks" {
		t.Errorf("PostPath = %q, want /accounts/9900001/sip-trunks", fake.PostPath)
	}
	body, ok := fake.PostBody.(map[string]interface{})
	if !ok || body["transport"] != "TLS" || len(body) != 1 {
		t.Errorf("PostBody = %#v, want {transport: TLS}", fake.PostBody)
	}
}

func TestSipTrunkNormalizesTransport(t *testing.T) {
	fake := &testutil.FakeClient{PostResult: map[string]interface{}{"data": map[string]interface{}{"state": "PROVISIONING"}}}
	swapTrunkClient(t, fake)

	if _, err := runTrunkCreateCmd(t, "--transport", " udp ", "--plain"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	body, _ := fake.PostBody.(map[string]interface{})
	if body["transport"] != "UDP" {
		t.Errorf("transport = %v, want UDP", body["transport"])
	}
}

func TestSipTrunkInvalidTransportIsFlagError(t *testing.T) {
	for _, args := range [][]string{{}, {"--transport", "tcp"}} {
		fake := &testutil.FakeClient{}
		swapTrunkClient(t, fake)

		_, err := runTrunkCreateCmd(t, args...)
		if err == nil {
			t.Fatalf("args %v: expected error", args)
		}
		if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
			t.Errorf("args %v: exit = %d, want %d", args, code, cmdutil.ExitFlagError)
		}
		if fake.PostPath != "" {
			t.Errorf("args %v: unexpected request to %q", args, fake.PostPath)
		}
	}
}

func TestSipTrunkConflictExitsFour(t *testing.T) {
	fake := &testutil.FakeClient{PostErr: &api.APIError{StatusCode: 409, Body: "conflict"}}
	swapTrunkClient(t, fake)

	_, err := runTrunkCreateCmd(t, "--transport", "udp", "--plain")
	if err == nil {
		t.Fatal("expected error")
	}
	var ce *cmdutil.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("error is %T, want *cmdutil.ConflictError", err)
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit = %d, want %d", code, cmdutil.ExitConflict)
	}
}
