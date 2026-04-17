package transcription

import (
	"testing"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "transcription" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "transcription")
	}

	subs := map[string]bool{}
	for _, c := range Cmd.Commands() {
		subs[c.Use] = true
	}
	for _, name := range []string{"create <callId> <recordingId>", "get <callId> <recordingId>"} {
		if !subs[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestCreateArgs(t *testing.T) {
	if createCmd.Args == nil {
		t.Fatal("create command should have arg validation")
	}
}

func TestCreateFlags(t *testing.T) {
	for _, flag := range []string{"wait", "timeout"} {
		f := createCmd.Flags().Lookup(flag)
		if f == nil {
			t.Errorf("missing flag %q", flag)
		}
	}
}
