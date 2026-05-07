package bulk

import (
	"testing"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "bulk" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "bulk")
	}
	expected := []string{"create", "get", "get-tns", "list"}
	have := map[string]bool{}
	for _, c := range Cmd.Commands() {
		have[c.Name()] = true
	}
	for _, want := range expected {
		if !have[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
}

// TestFlattenBulkResultLocksV1Shape verifies the bulk plain shape contract:
// {bulkOrderId, status, childOrderIds, portableNumbers, nonPortable}.
func TestFlattenBulkResultLocksV1Shape(t *testing.T) {
	resp := map[string]interface{}{
		"BulkPortinResponse": map[string]interface{}{
			"OrderId":          "bulk-abc",
			"ProcessingStatus": "INVALID_DRAFT_TNS",
			"PortableTnList": map[string]interface{}{
				"TN": []interface{}{"8336531000"},
			},
			"ChildPortinOrderList": map[string]interface{}{
				"ChildPortinOrder": map[string]interface{}{
					"OrderId": "child-1",
					"TnList": map[string]interface{}{
						"Tn": "8336531000",
					},
				},
			},
			"ErrorList": map[string]interface{}{
				"Error": map[string]interface{}{
					"Code":        "7642",
					"Description": "TN list contains at least one toll free number that cannot be ported due to spare status.",
					"TnList": map[string]interface{}{
						"Tn": "8005587721",
					},
				},
			},
		},
	}

	got := flattenBulkResult(resp)
	for _, k := range []string{"bulkOrderId", "status", "childOrderIds", "portableNumbers", "nonPortable"} {
		if _, ok := got[k]; !ok {
			t.Errorf("v1 plain shape missing key %q", k)
		}
	}
	if got["bulkOrderId"] != "bulk-abc" {
		t.Errorf("bulkOrderId = %v, want bulk-abc", got["bulkOrderId"])
	}
	children, _ := got["childOrderIds"].([]string)
	if len(children) == 0 || children[0] != "child-1" {
		t.Errorf("childOrderIds = %v, want [child-1]", children)
	}
	nonPortable, _ := got["nonPortable"].([]map[string]interface{})
	if len(nonPortable) == 0 {
		t.Fatal("expected at least one nonPortable entry")
	}
	if code, _ := nonPortable[0]["code"].(string); code != "7642" {
		t.Errorf("nonPortable[0].code = %q, want 7642", code)
	}
}

func TestStripE164(t *testing.T) {
	cases := []struct{ in, want string }{
		{"+18005551234", "8005551234"},
		{"18005551234", "8005551234"},
		{"8005551234", "8005551234"},
	}
	for _, tt := range cases {
		if got := stripE164(tt.in); got != tt.want {
			t.Errorf("stripE164(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
