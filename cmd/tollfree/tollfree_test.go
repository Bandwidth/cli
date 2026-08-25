package tollfree

import (
	"fmt"
	"testing"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "tollfree" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "tollfree")
	}

	subs := map[string]bool{}
	for _, c := range Cmd.Commands() {
		subs[c.Use] = true
	}
	if !subs["template <number> [number...]"] {
		t.Error("missing subcommand template")
	}
}

func TestNormalizeTollFreeNumbers(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr bool
	}{
		{
			name: "mixed input formats normalize to E.164",
			args: []string{"+18005551234", "8885551234", "18775551234", "(866) 555-1234"},
			want: []string{"+18005551234", "+18885551234", "+18775551234", "+18665551234"},
		},
		{
			name: "duplicates collapse, order preserved",
			args: []string{"8005551234", "+18005551234", "8885551234"},
			want: []string{"+18005551234", "+18885551234"},
		},
		{
			name:    "local number rejected",
			args:    []string{"+19195551234"},
			wantErr: true,
		},
		{
			name:    "short code rejected",
			args:    []string{"55512"},
			wantErr: true,
		},
		{
			name:    "non-NANP rejected",
			args:    []string{"+448005551234"},
			wantErr: true,
		},
		{
			name:    "vanity letters rejected",
			args:    []string{"800ABC-DEFG"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeTollFreeNumbers(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeTollFreeNumbers(%v) = %v, want error", tt.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeTollFreeNumbers(%v) error: %v", tt.args, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestNormalizeTollFreeNumbersLimit(t *testing.T) {
	args := make([]string, maxTemplateNumbers+1)
	for i := range args {
		// Distinct valid toll-free numbers: 800555XXXX.
		args[i] = fmt.Sprintf("+1800%07d", i)
	}
	if _, err := normalizeTollFreeNumbers(args); err == nil {
		t.Error("expected error for over-limit input")
	}
}

func TestTemplateSearchBody(t *testing.T) {
	body := templateSearchBody([]string{"+18005551234"})
	criteria, ok := body["queryCriteria"].([]map[string]interface{})
	if !ok || len(criteria) != 1 {
		t.Fatalf("queryCriteria = %#v, want single-entry slice", body["queryCriteria"])
	}
	c := criteria[0]
	if c["operator"] != "IN" || c["parameter"] != "phoneNumbers" {
		t.Errorf("criterion = %#v, want operator IN on phoneNumbers", c)
	}
	values, ok := c["values"].([]string)
	if !ok || len(values) != 1 || values[0] != "+18005551234" {
		t.Errorf("values = %#v, want [+18005551234]", c["values"])
	}
}

func TestUnwrapTemplateMappings(t *testing.T) {
	mappings := []interface{}{
		map[string]interface{}{"phoneNumber": "+18004329876", "templateName": "ATemplate", "reasonForNoTemplate": nil},
	}
	wrapped := map[string]interface{}{
		"data":   map[string]interface{}{"phoneNumberTemplateMappings": mappings},
		"errors": []interface{}{},
		"links":  []interface{}{},
	}
	got, ok := unwrapTemplateMappings(wrapped).([]interface{})
	if !ok || len(got) != 1 {
		t.Fatalf("unwrap = %#v, want the mappings array", unwrapTemplateMappings(wrapped))
	}

	// Unexpected shapes pass through untouched.
	odd := map[string]interface{}{"surprise": true}
	if got := unwrapTemplateMappings(odd); got == nil {
		t.Error("unexpected shape should pass through, not vanish")
	}
}
