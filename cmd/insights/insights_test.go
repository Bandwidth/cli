package insights

import (
	"testing"
	"time"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "insights" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "insights")
	}

	subs := map[string]bool{}
	for _, c := range Cmd.Commands() {
		subs[c.Use] = true
	}
	for _, name := range []string{"minutes-of-use", "completed-calls", "failed-calls", "connection-rates", "average-durations"} {
		if !subs[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

func TestParseTimeFlag(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "7d", want: "2026-08-17T12:00:00Z"},
		{input: "24h", want: "2026-08-23T12:00:00Z"},
		{input: "90m", want: "2026-08-24T10:30:00Z"},
		{input: "2026-07-01T00:00:00Z", want: "2026-07-01T00:00:00Z"},
		{input: "2026-07-01T00:00:00-05:00", want: "2026-07-01T00:00:00-05:00"},
		{input: "yesterday", wantErr: true},
		{input: "2026-07-01", wantErr: true},
		{input: "7w", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseTimeFlag(tt.input, now)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseTimeFlag(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTimeFlag(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseTimeFlag(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildMonitorQuery(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	q, err := buildMonitorQuery("9901303", monitorFlags{
		To:        "+18005551234,+18885551234",
		Direction: "inbound",
		CallType:  "tollfree-in",
		Since:     "30d",
	}, now)
	if err != nil {
		t.Fatalf("buildMonitorQuery error: %v", err)
	}
	want := map[string]string{
		"accountId[eq]":     "9901303",
		"toPhoneNumber[eq]": "+18005551234,+18885551234",
		"direction[eq]":     "INBOUND",
		"callType[eq]":      "TOLLFREE_IN",
		"timestamp[gte]":    "2026-07-25T12:00:00Z",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("q[%s] = %q, want %q", k, q.Get(k), v)
		}
	}
	if q.Get("timestamp[lte]") != "" {
		t.Error("timestamp[lte] should be unset when --until absent")
	}

	if _, err := buildMonitorQuery("1", monitorFlags{Direction: "SIDEWAYS"}, now); err == nil {
		t.Error("invalid direction should be a flag error")
	}
}

func TestNormalizeCallType(t *testing.T) {
	for input, want := range map[string]string{
		"TOLLFREE-IN": "TOLLFREE_IN",
		"tollfree_in": "TOLLFREE_IN",
		"local":       "LOCAL",
	} {
		if got := normalizeCallType(input); got != want {
			t.Errorf("normalizeCallType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUnwrapMonitorData(t *testing.T) {
	env := map[string]interface{}{
		"links":  []interface{}{},
		"data":   map[string]interface{}{"aggregation": "hourly", "slices": []interface{}{}},
		"errors": []interface{}{},
	}
	got, ok := unwrapMonitorData(env).(map[string]interface{})
	if !ok || got["aggregation"] != "hourly" {
		t.Errorf("unwrap = %#v, want data object", unwrapMonitorData(env))
	}
	odd := map[string]interface{}{"surprise": true}
	if unwrapMonitorData(odd) == nil {
		t.Error("unexpected shape should pass through")
	}
}
