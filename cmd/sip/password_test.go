package sip

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestReadPassword_FromFileStripsSingleTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pw")
	if err := os.WriteFile(p, []byte("s3cret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	pw, generated, err := readPassword(&cobra.Command{}, false, p, false)
	if err != nil {
		t.Fatalf("readPassword() error = %v", err)
	}
	if pw != "s3cret" {
		t.Errorf("password = %q, want s3cret", pw)
	}
	if generated {
		t.Error("generated = true, want false for caller-supplied password")
	}
}

func TestReadPassword_FromFileStripsCRLF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pw")
	os.WriteFile(p, []byte("s3cret\r\n"), 0600)
	pw, _, err := readPassword(&cobra.Command{}, false, p, false)
	if err != nil {
		t.Fatalf("readPassword() error = %v", err)
	}
	if pw != "s3cret" {
		t.Errorf("password = %q, want s3cret", pw)
	}
}

func TestReadPassword_RejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pw")
	os.WriteFile(p, []byte("\n"), 0600)
	if _, _, err := readPassword(&cobra.Command{}, false, p, false); err == nil {
		t.Error("empty password accepted, want error")
	}
}

func TestReadPassword_Generate(t *testing.T) {
	pw, generated, err := readPassword(&cobra.Command{}, false, "", true)
	if err != nil {
		t.Fatalf("readPassword() error = %v", err)
	}
	if len(pw) != 22 {
		t.Errorf("generated password length = %d, want 22", len(pw))
	}
	if !generated {
		t.Error("generated = false, want true")
	}
}

func TestReadPassword_RequiresExactlyOneSource(t *testing.T) {
	if _, _, err := readPassword(&cobra.Command{}, false, "", false); err == nil {
		t.Error("no password source accepted, want error")
	}
	if _, _, err := readPassword(&cobra.Command{}, true, "", true); err == nil {
		t.Error("multiple password sources accepted, want error")
	}
}

// TestReadPassword_FromStdinPipe covers the flow the create command's Example
// documents as recommended for agents: `printf '%s' "$SIP_PASSWORD" | band ...
// --password-stdin`. IsInteractive() reports false for the piped/redirected
// stdin a `go test` process runs under, so this exercises the same
// io.LimitReader(cmd.InOrStdin(), ...) branch a real pipe would take.
func TestReadPassword_FromStdinPipe(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("hunter2\n"))
	pw, generated, err := readPassword(cmd, true, "", false)
	if err != nil {
		t.Fatalf("readPassword() error = %v", err)
	}
	if pw != "hunter2" {
		t.Errorf("password = %q, want hunter2", pw)
	}
	if generated {
		t.Error("generated = true, want false for a piped, caller-supplied password")
	}
}

func TestReadPassword_MaxLengthPasswordWithTrailingNewlineIsAccepted(t *testing.T) {
	// Regression guard: the size cap must be enforced AFTER trimming the
	// trailing newline, not before — otherwise a legitimate maxPasswordBytes
	// password written by `echo` (which appends \n) is rejected.
	dir := t.TempDir()
	p := filepath.Join(dir, "pw")
	pwBytes := strings.Repeat("a", maxPasswordBytes)
	if err := os.WriteFile(p, []byte(pwBytes+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	pw, _, err := readPassword(&cobra.Command{}, false, p, false)
	if err != nil {
		t.Fatalf("readPassword() error = %v, want max-length password with trailing newline accepted", err)
	}
	if pw != pwBytes {
		t.Errorf("password length = %d, want %d", len(pw), maxPasswordBytes)
	}
}

func TestReadPassword_RejectsOversizedInput(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pw")
	os.WriteFile(p, []byte(strings.Repeat("a", 2048)), 0600)
	if _, _, err := readPassword(&cobra.Command{}, false, p, false); err == nil {
		t.Error("oversized password accepted, want error")
	}
}
