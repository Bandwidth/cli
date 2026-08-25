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
		// UNVERIFIED is pending, not failure — this contradicts
		// enumBrandIdentityStatus, which documents REGISTERING as the
		// in-progress state. Measured on production, 2026-08-19: production
		// never returns REGISTERING on the read path; every brand reads
		// UNVERIFIED from the moment its 202 lands. Two byte-identical
		// submissions both read UNVERIFIED at t≈3s — one (WOVNQBAVI2) went on
		// to VERIFIED at t≈46s, the other (WAR2FRJPVQ) was still UNVERIFIED
		// at t≈275s with no TCR response in its history at all. Do not flip
		// this back to StateFailure to match the enum: that would restore the
		// bug where 'brand create --wait' exits 4 within seconds for every
		// newly created brand, including ones that go on to verify. See
		// ClassifyBrandIdentity's comment for the full evidence.
		{"UNVERIFIED", StatePending},
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
	errText := BrandRemediation("ERROR")
	if errText == "" {
		t.Fatal("ERROR must have remediation text")
	}
	if !strings.Contains(errText, "refresh") {
		t.Errorf("ERROR remediation should mention %q, got: %s", "refresh", errText)
	}
	// UNVERIFIED is no longer classified as a business failure (see
	// ClassifyBrandIdentity) — production uses it for both "still
	// registering" and "rejected", and this package can't tell which. It must
	// not have failure-remediation text of its own; the ambiguity is
	// surfaced instead through the timeout receipt's lastSeenStatus and note.
	if got := BrandRemediation("UNVERIFIED"); got != "" {
		t.Errorf("UNVERIFIED is no longer a business failure, want no remediation, got: %s", got)
	}
	if BrandRemediation("VERIFIED") != "" {
		t.Error("a success state has no remediation")
	}
}

func TestClassifyCampaignStatus(t *testing.T) {
	tests := []struct {
		status string
		want   StateClass
	}{
		{"REGISTERED", StateSuccess},
		{"DECLINED", StateFailure},
		{"EXPIRED", StateFailure},
		{"SUSPENDED", StateFailure},
		{"ERROR", StateFailure},
		// Unlike brand identity, campaigns genuinely return REGISTERING on
		// the read path — it's an in-progress state, not a stand-in for an
		// unknown terminal outcome.
		{"REGISTERING", StatePending},
		{"UNREGISTERING", StatePending},
		// An unlisted value keeps polling until timeout rather than being
		// guessed as success or failure.
		{"SOMETHING_NEW", StatePending},
		{"", StatePending},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := ClassifyCampaignStatus(tt.status); got != tt.want {
				t.Errorf("ClassifyCampaignStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

// A business failure that says only "it failed" wastes the operator's next
// step. Each failure state names what to do about it, and the wording must
// actually differ between states — a function returning the same sentence
// for every failure would still pass a check that only asserts non-empty.
func TestCampaignRemediationIsSpecificPerState(t *testing.T) {
	declined := CampaignRemediation("DECLINED")
	expired := CampaignRemediation("EXPIRED")
	suspended := CampaignRemediation("SUSPENDED")
	errText := CampaignRemediation("ERROR")

	for name, text := range map[string]string{
		"DECLINED":  declined,
		"EXPIRED":   expired,
		"SUSPENDED": suspended,
		"ERROR":     errText,
	} {
		if text == "" {
			t.Errorf("%s must have remediation text", name)
		}
	}

	if !strings.Contains(declined, "nudge") || !strings.Contains(declined, "APPEAL_REJECTION") {
		t.Errorf("DECLINED remediation should mention the nudge appeal path, got: %s", declined)
	}
	if !strings.Contains(expired, "Re-create") {
		t.Errorf("EXPIRED remediation should mention re-creating the campaign, got: %s", expired)
	}
	if !strings.Contains(suspended, "account manager") {
		t.Errorf("SUSPENDED remediation should mention the account manager, got: %s", suspended)
	}
	if !strings.Contains(errText, "sync") {
		t.Errorf("ERROR remediation should mention %q, got: %s", "sync", errText)
	}

	texts := []string{declined, expired, suspended, errText}
	for i := range texts {
		for j := i + 1; j < len(texts); j++ {
			if texts[i] == texts[j] {
				t.Errorf("remediation text for distinct failure states must not be identical: %q", texts[i])
			}
		}
	}

	for _, status := range []string{"REGISTERED", "REGISTERING", "UNREGISTERING", "SOMETHING_NEW", ""} {
		if got := CampaignRemediation(status); got != "" {
			t.Errorf("CampaignRemediation(%q) = %q, want empty (success/pending state)", status, got)
		}
	}
}
