package recording

import (
	"testing"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "recording" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "recording")
	}

	subs := map[string]bool{}
	for _, c := range Cmd.Commands() {
		subs[c.Use] = true
	}
	for _, name := range []string{"get <callId> <recordingId>", "list <callId>", "delete <callId> <recordingId>", "download <callId> <recordingId>", "pause <callId>", "resume <callId>"} {
		if !subs[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestGetArgs(t *testing.T) {
	if getCmd.Args == nil {
		t.Fatal("get command should have arg validation")
	}
}

func TestDeleteArgs(t *testing.T) {
	if deleteCmd.Args == nil {
		t.Fatal("delete command should have arg validation")
	}
}

func TestDownloadRequiredFlags(t *testing.T) {
	f := downloadCmd.Flags().Lookup("output")
	if f == nil {
		t.Error("missing flag \"output\"")
	}
}

func TestPauseArgs(t *testing.T) {
	if pauseCmd.Args == nil {
		t.Fatal("pause command should have arg validation")
	}
}

func TestResumeArgs(t *testing.T) {
	if resumeCmd.Args == nil {
		t.Fatal("resume command should have arg validation")
	}
}
