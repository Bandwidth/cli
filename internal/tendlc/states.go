package tendlc

// StateClass is how a polled resource's status is interpreted: keep waiting,
// stop and succeed, or stop and report a business failure.
type StateClass int

const (
	// StatePending means keep polling. It is the DEFAULT for any value not
	// explicitly classified, which matters: production returns statuses that
	// appear in no published enum. Treating an unknown state as pending costs
	// a timeout; treating it as success reports a failed registration as done.
	StatePending StateClass = iota
	StateSuccess
	StateFailure
)

// ClassifyBrandIdentity maps brandIdentityStatus to a polling outcome.
// Derived from statuses observed live across 10 brands, not from the enum.
func ClassifyBrandIdentity(status string) StateClass {
	switch status {
	case "VERIFIED", "VETTED_VERIFIED", "SELF_DECLARED":
		return StateSuccess
	case "UNVERIFIED", "ERROR":
		return StateFailure
	default:
		return StatePending
	}
}

// ClassifyVetting maps a brand vetting's vettingStatus to a polling outcome.
//
// The brand-vetting enum is NOT the campaign vettingStatus enum. Live vettings
// report ACTIVE, which is documented in neither.
func ClassifyVetting(status string) StateClass {
	switch status {
	case "ACTIVE":
		return StateSuccess
	case "FAILED", "EXPIRED":
		return StateFailure
	default:
		return StatePending
	}
}

// BrandRemediation returns what to do about a brand that settled into a
// business-failure state, or "" for any state that is not one.
func BrandRemediation(status string) string {
	switch status {
	case "UNVERIFIED":
		return "identity could not be confirmed — most often the legal company name does not match the EIN. " +
			"Correct the details with 'band tendlc brand update' then 'band tendlc brand reverify' (incurs a $4 fee), " +
			"or request external vetting with 'band tendlc vetting request'."
	case "ERROR":
		return "the registry reported an error on this brand. Re-pull its current state from TCR with " +
			"'band tendlc brand refresh', and contact your Bandwidth account manager if it persists."
	default:
		return ""
	}
}
