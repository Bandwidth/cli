package account

import (
	"testing"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "account" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "account")
	}

	subs := map[string]bool{}
	for _, c := range Cmd.Commands() {
		subs[c.Use] = true
	}
	if !subs["register"] {
		t.Errorf("missing subcommand %q", "register")
	}
}

func TestRegisterRequiredFlags(t *testing.T) {
	for _, flag := range []string{"phone", "email", "first-name", "last-name"} {
		f := registerCmd.Flags().Lookup(flag)
		if f == nil {
			t.Errorf("missing flag %q", flag)
			continue
		}
		ann := registerCmd.Flags().Lookup(flag).Annotations
		if _, ok := ann["cobra_annotation_bash_completion_one_required_flag"]; !ok {
			t.Errorf("flag %q should be required", flag)
		}
	}
}
