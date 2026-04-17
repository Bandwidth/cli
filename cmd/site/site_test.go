package site

import (
	"testing"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "subaccount" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "subaccount")
	}

	if len(Cmd.Aliases) == 0 || Cmd.Aliases[0] != "site" {
		t.Error("expected \"site\" alias")
	}

	subs := map[string]bool{}
	for _, c := range Cmd.Commands() {
		subs[c.Use] = true
	}
	for _, name := range []string{"create", "delete [id]", "get [id]", "list"} {
		if !subs[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestCreateRequiredFlags(t *testing.T) {
	f := createCmd.Flags().Lookup("name")
	if f == nil {
		t.Error("missing flag \"name\"")
	}
}

func TestCreateOptionalFlags(t *testing.T) {
	for _, flag := range []string{"description", "if-not-exists"} {
		f := createCmd.Flags().Lookup(flag)
		if f == nil {
			t.Errorf("missing flag %q", flag)
		}
	}
}
