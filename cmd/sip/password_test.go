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

func TestReadPassword_RejectsOversizedInput(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pw")
	os.WriteFile(p, []byte(strings.Repeat("a", 2048)), 0600)
	if _, _, err := readPassword(&cobra.Command{}, false, p, false); err == nil {
		t.Error("oversized password accepted, want error")
	}
}
