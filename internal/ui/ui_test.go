package ui

import (
	"os"
	"strings"
	"testing"
)

func TestNewSpinner(t *testing.T) {
	s := NewSpinner("Loading...")
	if s == nil {
		t.Fatal("NewSpinner returned nil")
	}
	// Spinner should write to stderr, not stdout
	if s.Writer != os.Stderr {
		t.Error("spinner writer should be os.Stderr")
	}
	if !strings.Contains(s.Suffix, "Loading...") {
		t.Errorf("spinner suffix = %q, want it to contain 'Loading...'", s.Suffix)
	}
}

func TestColorFunctions(t *testing.T) {
	// These return SprintFunc closures — verify they produce non-empty output
	// and don't panic.
	tests := []struct {
		name string
		fn   func(a ...interface{}) string
	}{
		{"Success", Success},
		{"Error", Error},
		{"Warn", Warn},
		{"Muted", Muted},
		{"Bold", Bold},
		{"ID", ID},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn("test")
			if got == "" {
				t.Errorf("%s(\"test\") returned empty string", tc.name)
			}
			// The raw text should be present somewhere in the output
			// (possibly wrapped with ANSI codes)
			if !strings.Contains(got, "test") {
				t.Errorf("%s(\"test\") = %q, doesn't contain 'test'", tc.name, got)
			}
		})
	}
}
