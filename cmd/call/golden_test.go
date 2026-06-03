package call

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/testutil"
)

func TestCallListPlainOutput(t *testing.T) {
	// Fixture is an API-shaped WRAPPER object ({"calls": [...]}), so a passing
	// assertion on got[0]["callId"] proves FlattenResponse stripped the wrapper.
	// No t.Parallel(): these tests mutate the global cmdutil.VoiceClient.
	orig := cmdutil.VoiceClient
	t.Cleanup(func() { cmdutil.VoiceClient = orig })
	cmdutil.VoiceClient = func(string) (api.Requester, string, error) {
		return &testutil.FakeClient{GetResult: map[string]interface{}{
			"calls": []interface{}{
				map[string]interface{}{"callId": "c-1", "state": "active"},
			},
		}}, "acct-123", nil
	}

	root := testutil.NewTestRoot(listCmd)
	root.SetArgs([]string{"list", "--plain"})

	out := testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var got []map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &got); err != nil {
		t.Fatalf("plain output is not a JSON array: %q (%v)", out, err)
	}
	if len(got) != 1 || got[0]["callId"] != "c-1" {
		t.Fatalf("flatten/normalize did not produce the expected array: %q", out)
	}

	want := "[\n  {\n    \"callId\": \"c-1\",\n    \"state\": \"active\"\n  }\n]\n"
	if out != want {
		t.Fatalf("golden mismatch:\n got: %q\nwant: %q", out, want)
	}
}
