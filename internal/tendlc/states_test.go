package tendlc

import (
	"strings"
	"testing"
)

func TestClassifyBrandIdentity(t *testing.T) {
	tests := []struct {
		status string
		want   StateClass
	}{
		{"VERIFIED", StateSuccess},
		{"VETTED_VERIFIED", StateSuccess},
		{"SELF_DECLARED", StateSuccess},
		{"UNVERIFIED", StateFailure},
		{"ERROR", StateFailure},
		{"REGISTERING", StatePending},
		// An unlisted value keeps polling until timeout. It is never reported
		// as success: the vetting enum already surprised us with a live value
		// documented nowhere, and guessing "probably fine" on an unknown
		// terminal state is how a CLI reports a failed registration as done.
		{"SOMETHING_NEW", StatePending},
		{"", StatePending},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := ClassifyBrandIdentity(tt.status); got != tt.want {
				t.Errorf("ClassifyBrandIdentity(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestClassifyVetting(t *testing.T) {
	tests := []struct {
		status string
		want   StateClass
	}{
		// ACTIVE is what live brand vettings report. It appears in neither
		// documented enum.
		{"ACTIVE", StateSuccess},
		{"FAILED", StateFailure},
		{"EXPIRED", StateFailure},
		{"PENDING", StatePending},
		{"UNSCORE", StatePending},
		{"SOMETHING_NEW", StatePending},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := ClassifyVetting(tt.status); got != tt.want {
				t.Errorf("ClassifyVetting(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

// A business failure that says only "it failed" wastes the operator's next
// step. Each failure state names what to do about it.
func TestBrandRemediationIsSpecificPerState(t *testing.T) {
	unverified := BrandRemediation("UNVERIFIED")
	if unverified == "" {
		t.Fatal("UNVERIFIED must have remediation text")
	}
	for _, want := range []string{"reverify", "vetting"} {
		if !strings.Contains(unverified, want) {
			t.Errorf("UNVERIFIED remediation should mention %q, got: %s", want, unverified)
		}
	}
	if BrandRemediation("ERROR") == unverified {
		t.Error("ERROR and UNVERIFIED must not share the same remediation text")
	}
	if BrandRemediation("VERIFIED") != "" {
		t.Error("a success state has no remediation")
	}
}
