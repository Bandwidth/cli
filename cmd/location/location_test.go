package location

import (
	"testing"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "location" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "location")
	}

	subs := map[string]bool{}
	for _, c := range Cmd.Commands() {
		subs[c.Use] = true
	}
	for _, name := range []string{"create", "list"} {
		if !subs[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestCreateRequiredFlags(t *testing.T) {
	for _, flag := range []string{"site", "name"} {
		f := createCmd.Flags().Lookup(flag)
		if f == nil {
			t.Errorf("missing flag %q", flag)
		}
	}
}

func TestListRequiredFlags(t *testing.T) {
	f := listCmd.Flags().Lookup("site")
	if f == nil {
		t.Error("missing flag \"site\"")
	}
}
