package recording

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
)

// fakeClient implements api.Requester. Get writes a canned fixture.
type fakeClient struct {
	getResult interface{}
}

func (f *fakeClient) Get(path string, result interface{}) error {
	b, _ := json.Marshal(f.getResult)
	return json.Unmarshal(b, result)
}
func (f *fakeClient) Post(string, interface{}, interface{}) error  { return nil }
func (f *fakeClient) Put(string, interface{}, interface{}) error   { return nil }
func (f *fakeClient) Patch(string, interface{}, interface{}) error { return nil }
func (f *fakeClient) Delete(string, interface{}) error             { return nil }
func (f *fakeClient) GetRaw(string) ([]byte, error)                { return nil, nil }
func (f *fakeClient) PutRaw(string, []byte, string) error          { return nil }

// newTestRoot builds a minimal root with the persistent flags commands read.
func newTestRoot(child *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "band", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Bool("plain", false, "")
	root.PersistentFlags().String("account-id", "", "")
	root.PersistentFlags().String("environment", "", "")
	root.AddCommand(child)
	return root
}

// captureStdout runs fn while capturing everything written to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = orig
	var out []byte
	out, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestRecordingListPlainOutput(t *testing.T) {
	// No t.Parallel(): these tests mutate the global cmdutil.VoiceClient.
	orig := cmdutil.VoiceClient
	t.Cleanup(func() { cmdutil.VoiceClient = orig })
	cmdutil.VoiceClient = func(string) (api.Requester, string, error) {
		return &fakeClient{getResult: map[string]interface{}{
			"recordings": []interface{}{
				map[string]interface{}{"recordingId": "r-1", "status": "complete"},
			},
		}}, "acct-123", nil
	}

	root := newTestRoot(listCmd)
	root.SetArgs([]string{"list", "c-abc123", "--plain"}) // callId positional arg required

	out := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var got []map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &got); err != nil {
		t.Fatalf("plain output is not a JSON array: %q (%v)", out, err)
	}
	if len(got) != 1 || got[0]["recordingId"] != "r-1" {
		t.Fatalf("flatten/normalize did not produce the expected array: %q", out)
	}

	want := "[\n  {\n    \"recordingId\": \"r-1\",\n    \"status\": \"complete\"\n  }\n]\n"
	if out != want {
		t.Fatalf("golden mismatch:\n got: %q\nwant: %q", out, want)
	}
}
