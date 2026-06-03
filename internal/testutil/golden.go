// Package testutil provides shared helpers for command-level golden tests:
// a fake api.Requester, a minimal root command with the persistent flags
// commands read, and an os.Stdout capture. It lives in a regular package (not a
// _test.go file) so multiple command packages can share it; only _test.go files
// import it, so it is never linked into the production binary.
package testutil

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

// FakeClient implements api.Requester. Get marshals GetResult into the caller's
// result pointer (a JSON round-trip), mimicking a real API response; the other
// methods are no-ops. Set GetResult to the canned fixture for the command.
type FakeClient struct {
	GetResult interface{}
}

func (f *FakeClient) Get(path string, result interface{}) error {
	b, _ := json.Marshal(f.GetResult)
	return json.Unmarshal(b, result)
}
func (f *FakeClient) Post(string, interface{}, interface{}) error  { return nil }
func (f *FakeClient) Put(string, interface{}, interface{}) error   { return nil }
func (f *FakeClient) Patch(string, interface{}, interface{}) error { return nil }
func (f *FakeClient) Delete(string, interface{}) error             { return nil }
func (f *FakeClient) GetRaw(string) ([]byte, error)                { return nil, nil }
func (f *FakeClient) PutRaw(string, []byte, string) error          { return nil }

// NewTestRoot builds a minimal root command carrying the persistent flags that
// command implementations read via cmd.Root().Flag(...), with child attached.
func NewTestRoot(child *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "band", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().String("format", "json", "")
	root.PersistentFlags().Bool("plain", false, "")
	root.PersistentFlags().String("account-id", "", "")
	root.PersistentFlags().String("environment", "", "")
	root.AddCommand(child)
	return root
}

// CaptureStdout runs fn while capturing everything written to os.Stdout.
func CaptureStdout(t *testing.T, fn func()) string {
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
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
