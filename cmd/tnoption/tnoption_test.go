package tnoption

import (
	"testing"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "tnoption" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "tnoption")
	}

	subs := map[string]bool{}
	for _, c := range Cmd.Commands() {
		subs[c.Use] = true
	}
	for _, name := range []string{"assign <number> [number...]", "get <orderId>", "list"} {
		if !subs[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestAssignRequiredFlags(t *testing.T) {
	f := assignCmd.Flags().Lookup("campaign-id")
	if f == nil {
		t.Fatal("missing --campaign-id flag")
	}
	ann := f.Annotations
	if _, ok := ann["cobra_annotation_bash_completion_one_required_flag"]; !ok {
		t.Error("--campaign-id should be required")
	}
}

func TestStripE164(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"+19195551234", "9195551234"},
		{"19195551234", "9195551234"},
		{"9195551234", "9195551234"},
		{"+449195551234", "449195551234"},
	}
	for _, tt := range tests {
		if got := stripE164(tt.input); got != tt.want {
			t.Errorf("stripE164(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDigString(t *testing.T) {
	resp := map[string]interface{}{
		"TnOptionOrderResponse": map[string]interface{}{
			"TnOptionOrder": map[string]interface{}{
				"OrderId":          "abc-123",
				"ProcessingStatus": "COMPLETE",
			},
		},
	}

	if got := digString(resp, "OrderId"); got != "abc-123" {
		t.Errorf("digString(OrderId) = %q, want %q", got, "abc-123")
	}
	if got := digString(resp, "ProcessingStatus"); got != "COMPLETE" {
		t.Errorf("digString(ProcessingStatus) = %q, want %q", got, "COMPLETE")
	}
	if got := digString(resp, "Missing"); got != "" {
		t.Errorf("digString(Missing) = %q, want empty", got)
	}
}
