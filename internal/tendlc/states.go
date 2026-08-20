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
//
// UNVERIFIED classifies as StatePending, not StateFailure — this looks wrong
// against enumBrandIdentityStatus, which documents REGISTERING as the
// in-progress state. It is not wrong. Measured on production, 2026-08-19:
// production never returns REGISTERING on the read path at all. Two brands
// submitted with byte-identical payloads both read UNVERIFIED within ~3s of
// their 202. WOVNQBAVI2 flipped to VERIFIED at t≈46s. WAR2FRJPVQ was still
// UNVERIFIED at t≈275s, with no TCR response in its history at all. So
// UNVERIFIED means either "TCR hasn't answered yet" or "TCR rejected it", and
// the status alone cannot say which — polling GET .../history for the
// free-text BRAND_IDENTITY_STATUS_UPDATE entry was considered and rejected,
// since that message is undocumented and unversioned and coupling poll
// control flow to its wording would be worse than the latency. Classifying
// UNVERIFIED as a failure made 'brand create --wait' exit 4 within seconds of
// every create, including brands that went on to verify — a false failure on
// the flagship async path, on every single run. Do not "correct" this back
// to StateFailure to match the published enum: the enum is aspirational,
// this is what production actually does. The accepted tradeoff is that a
// brand that really did fail now polls to timeout instead of failing fast;
// awaitTerminal's last-seen-status mechanism exists so that timeout receipt
// still tells the caller what was last observed.
func ClassifyBrandIdentity(status string) StateClass {
	switch status {
	case "VERIFIED", "VETTED_VERIFIED", "SELF_DECLARED":
		return StateSuccess
	case "ERROR":
		return StateFailure
	case "UNVERIFIED":
		// Explicit rather than falling through to default: this is the one
		// value most likely to be "corrected" back to StateFailure by a
		// future reader who only checks it against the enum. See the
		// evidence above.
		return StatePending
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

// ClassifyCampaignStatus maps a campaign's status to a polling outcome.
//
// Unlike ClassifyBrandIdentity, this does NOT treat any failure-looking value
// as pending. Campaigns genuinely return REGISTERING on the read path while
// registration is in progress — it is in the documented enum and has been
// observed live — so REGISTERING/UNREGISTERING are unambiguous in-progress
// states, and DECLINED/EXPIRED/SUSPENDED/ERROR are unambiguous terminal
// failures. The brand-side UNVERIFIED workaround exists specifically because
// brands do NOT return REGISTERING on their read path, making a registering
// brand indistinguishable from a rejected one by status alone; that ambiguity
// does not exist here. Do not "harmonize" this classifier with
// ClassifyBrandIdentity by making campaign failure states pending — that
// would reintroduce the false-success risk this function exists to avoid.
func ClassifyCampaignStatus(status string) StateClass {
	switch status {
	case "REGISTERED":
		return StateSuccess
	case "DECLINED", "EXPIRED", "SUSPENDED", "ERROR":
		return StateFailure
	default:
		// Covers REGISTERING, UNREGISTERING, and any unlisted value. Unknown
		// values keep polling rather than being guessed as success, for the
		// same reason as ClassifyBrandIdentity's default: a false success
		// reports a failed registration as done, while a false pending only
		// costs a timeout.
		return StatePending
	}
}

// BrandRemediation returns what to do about a brand that settled into a
// business-failure state, or "" for any state that is not one.
//
// UNVERIFIED is deliberately absent: ClassifyBrandIdentity no longer treats
// it as a business failure (see its comment), so awaitTerminal never calls
// this with "UNVERIFIED" on the failure path. It also has no single
// remediation to give — it means either "still registering" or "rejected",
// and advising a paid reverify for a brand that may simply not have finished
// yet would be actively wrong. A brand stuck at UNVERIFIED past --wait's
// timeout is surfaced instead via the timeout receipt's lastSeenStatus and
// note, pointing at 'band tendlc brand get' / 'brand history' to check.
func BrandRemediation(status string) string {
	switch status {
	case "ERROR":
		return "the registry reported an error on this brand. Re-pull its current state from TCR with " +
			"'band tendlc brand refresh', and contact your Bandwidth account manager if it persists."
	default:
		return ""
	}
}

// CampaignRemediation returns what to do about a campaign that settled into
// a business-failure state, or "" for any state that is not one (including
// success and pending states — see ClassifyCampaignStatus).
func CampaignRemediation(status string) string {
	switch status {
	case "DECLINED":
		return "the campaign was rejected. An appeal may be possible: run " +
			"'band tendlc campaign nudge <campaign-id> --intent APPEAL_REJECTION'."
	case "EXPIRED":
		return "the registration lapsed and cannot be revived with a nudge. Re-create the campaign."
	case "SUSPENDED":
		return "the campaign was suspended by a carrier or the registry. This is not self-service — " +
			"contact your Bandwidth account manager."
	case "ERROR":
		return "the registry reported an error on this campaign. Re-pull its current state with " +
			"'band tendlc campaign sync <campaign-id>', and contact your Bandwidth account manager if it persists."
	default:
		return ""
	}
}
