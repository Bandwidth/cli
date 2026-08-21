package tendlc

import (
	"sort"
	"strings"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// CampaignUsecases are the usecases the API accepts for direct campaigns.
// Verified against enumCampaignUsecases in tendlc.yml, not transcribed from
// memory: K12_EDUCATION, TRIAL, AGENTS_FRANCHISES, UCAAS_HIGH, UCAAS_LOW, and
// M2M are real enum values that a memory-only list would have dropped.
var CampaignUsecases = []string{
	"2FA", "ACCOUNT_NOTIFICATION", "CARRIER_EXEMPT", "CHARITY", "CONVERSATIONAL",
	"CUSTOMER_CARE", "DELIVERY_NOTIFICATION", "EMERGENCY", "FRAUD_ALERT",
	"HIGHER_EDUCATION", "K12_EDUCATION", "LOW_VOLUME", "MARKETING", "MIXED",
	"POLITICAL", "POLLING_VOTING", "PUBLIC_SERVICE_ANNOUNCEMENT", "SECURITY_ALERT",
	"SOCIAL", "SWEEPSTAKE", "TRIAL", "AGENTS_FRANCHISES", "PROXY", "UCAAS_HIGH",
	"UCAAS_LOW", "M2M",
}

// CampaignSubUsecases are the sub-usecases the API accepts, per
// enumCampaignSubUsecases. The per-usecase count rules the spec describes
// (1-5 for Low Volume Mixed and Carrier Exemptions, 2-5 for Mixed, 0-5 for
// Proxy/Social/Charity/Political) are unverified against production and are
// deliberately not enforced here — only membership in this list is checked.
var CampaignSubUsecases = []string{
	"2FA", "ACCOUNT_NOTIFICATION", "CUSTOMER_CARE", "DELIVERY_NOTIFICATION",
	"FRAUD_ALERT", "HIGHER_EDUCATION", "MARKETING", "POLLING_VOTING",
	"PUBLIC_SERVICE_ANNOUNCEMENT", "SECURITY_ALERT",
}

// NudgeIntents are the valid values for a campaign nudge's intent field, per
// enumNudgeIntent.
var NudgeIntents = []string{"APPEAL_REJECTION", "REVIEW"}

// CampaignCreateOptions is the flag surface of `band tendlc campaign create`.
//
// Field-to-flag naming is mechanical: MessageFlow is --message-flow. Booleans
// have no zero-value meaning "unset" here — BuildCampaignCreateRequest reads
// intent from a changed map, not from these fields directly.
type CampaignCreateOptions struct {
	BrandID      string
	CampaignName string
	Usecase      string
	SubUsecases  []string
	Description  string
	Sample1      string
	Sample2      string
	Sample3      string
	Sample4      string
	Sample5      string
	MessageFlow  string

	HelpMessage            string
	HelpKeywords           string
	OptinMessage           string
	OptinKeywords          string
	OptoutMessage          string
	OptoutKeywords         string
	PrivacyPolicyLink      string
	TermsAndConditionsLink string
	EmbeddedLinkSample     string

	EmbeddedLink       bool
	EmbeddedPhone      bool
	TermsAndConditions bool
	NumberPool         bool
	AgeGated           bool
	DirectLending      bool
	SubscriberOptin    bool
	SubscriberOptout   bool
	SubscriberHelp     bool
	AutoRenewal        bool
}

// ValidateCampaignCreate checks the flags against a four-tier conditional
// requirement tree, returning every violation in one error, the way the API
// reports every violation in one 400.
//
// The table below was measured by creating a real campaign against
// production on 2026-08-21, not read from the schema — the spec's
// createCampaignRequest lists 7 required fields (brandId, usecase,
// description, sample1, messageFlow, helpMessage, helpKeywords) and is wrong
// in both directions: helpMessage/helpKeywords are not actually required
// (see the advisory return value below), and three more required fields
// only appear once tier 1 is satisfied.
//
//	Tier | Revealed when           | Fields
//	-----|--------------------------|----------------------------------------
//	1    | empty body               | brandId, usecase, description, sample1,
//	     |                          | messageFlow
//	2    | tier 1 satisfied         | subscriberOptin, subscriberOptout,
//	     |                          | subscriberHelp — all three required,
//	     |                          | and only true is an accepted value
//	3    | subscriberOptin: true    | optinMessage, optinKeywords
//	4    | subscriberOptout: true   | optoutMessage, optoutKeywords
//
// An earlier measurement concluded there were only 5 required fields and no
// second tier; that was a measurement artifact, not a production change. The
// probe used a deliberately-too-short description, and that length violation
// short-circuited every tier behind it — the same short-circuit hazard this
// validator routes around for --usecase below.
//
// Tier 2 is three booleans that default to false — exactly the kind of value
// BuildCampaignCreateRequest otherwise treats as "never sent" when the flag
// was never passed (see its doc comment). That omission is what keeps an
// explicit --age-gated=false distinguishable from silence on update. On
// create, these same three fields can be REQUIRED, so tier 2 is checked
// against the changed map, not against o's booleans directly: an unset
// --subscriber-optin and an explicitly-passed --subscriber-optin=false are
// different inputs, and only the former is "missing".
//
// The latter is its own, different violation: production accepts ONLY true
// for these three. Measured directly — a create with all three explicitly
// passed as false serialized to
//
//	{"brandId":"B","description":"d","messageFlow":"m","sample1":"s",
//	 "subscriberHelp":false,"subscriberOptin":false,"subscriberOptout":false,
//	 "usecase":"CUSTOMER_CARE"}
//
// (all three present, not omitted) and still came back
//
//	400 {"description":"is required","source":{"POINTER":"/subscriberOptin"}}
//	    {"description":"is required","source":{"POINTER":"/subscriberOptout"}}
//	    {"description":"is required","source":{"POINTER":"/subscriberHelp"}}
//
// i.e. the API treats an explicit false on these three the same as absent,
// and reports it as "is required" rather than as an invalid value — which
// reads exactly like a client-side serialization bug (a field silently
// dropped) even though the field was on the wire. A campaign apparently
// cannot declare "no opt-in, no opt-out, no help support"; only true is
// accepted. So an explicit false on any of the three is rejected here, with
// a message that names the real constraint instead of letting the caller
// chase a phantom "we forgot to send it" theory. This is NOT extended to any
// other boolean: --age-gated=false and the rest are legitimately settable on
// create, and this false-rejection is unique to these three tier-2
// attestations on the CREATE path (update is handled separately).
//
// Tiers 3 and 4 are conditional on the submitted VALUE (true), not on the
// flag's mere presence — checked as changed[flag] && o.Field, so neither an
// unset --subscriber-optin (zero value false) nor an explicit
// --subscriber-optin=false spuriously demands optinMessage/optinKeywords.
//
// subscriberHelp does not gate anything further: helpMessage/helpKeywords
// were supplied on the one measured create that succeeded, but nothing about
// that create proved they were required, so they remain the non-fatal
// advisory below rather than a tier-5 requirement invented from a single
// data point.
//
// --usecase is validated against CampaignUsecases. Production reports an
// unrecognized usecase as "is not allowed for direct campaigns" rather than
// "invalid value", and that error SHORT-CIRCUITS every other violation in
// the same request — a typo is indistinguishable from a permission boundary
// there. That is exactly why this validates client-side: an invalid usecase
// here does not short-circuit anything either, and is aggregated with every
// other violation into one error instead.
//
// --sub-usecase values are validated against CampaignSubUsecases the same
// way, also aggregated rather than short-circuiting.
//
// The second return value is a non-fatal advisory, empty when both
// helpMessage and helpKeywords are present. A HELP response is a carrier
// compliance expectation and TCR may decline a campaign without one, but
// rejecting a create over a field production does not require would be the
// exact failure mode this validator exists to avoid — a check that rejects
// what the API accepts. Silently letting a compliance gap through is also
// bad, so the gap is surfaced as an advisory the caller prints instead of a
// blocking error.
func ValidateCampaignCreate(o CampaignCreateOptions, changed map[string]bool) (string, error) {
	required := [][2]string{
		{"brand-id", o.BrandID},
		{"usecase", o.Usecase},
		{"description", o.Description},
		{"sample1", o.Sample1},
		{"message-flow", o.MessageFlow},
	}
	var missing []string
	for _, p := range required {
		if p[1] == "" {
			missing = append(missing, p[0])
		}
	}

	// Tier 2: required regardless of value, but only once explicitly passed —
	// see the doc comment above for why this reads changed instead of o. An
	// explicitly-passed false is a DIFFERENT violation from unset: production
	// accepts only true for these three (see the doc comment's wire-level
	// evidence), so false is collected separately rather than folded into
	// "missing".
	var mustBeTrue []string
	for _, tf := range []struct {
		flag  string
		value bool
	}{
		{"subscriber-optin", o.SubscriberOptin},
		{"subscriber-optout", o.SubscriberOptout},
		{"subscriber-help", o.SubscriberHelp},
	} {
		switch {
		case !changed[tf.flag]:
			missing = append(missing, tf.flag)
		case !tf.value:
			mustBeTrue = append(mustBeTrue, tf.flag)
		}
	}

	// Tiers 3 and 4: required only when the corresponding tier-2 flag was
	// explicitly passed AND its value is true.
	if changed["subscriber-optin"] && o.SubscriberOptin {
		if o.OptinMessage == "" {
			missing = append(missing, "optin-message")
		}
		if o.OptinKeywords == "" {
			missing = append(missing, "optin-keywords")
		}
	}
	if changed["subscriber-optout"] && o.SubscriberOptout {
		if o.OptoutMessage == "" {
			missing = append(missing, "optout-message")
		}
		if o.OptoutKeywords == "" {
			missing = append(missing, "optout-keywords")
		}
	}

	invalidUsecase := o.Usecase != "" && !validCampaignUsecase(o.Usecase)

	var invalidSubUsecases []string
	for _, su := range o.SubUsecases {
		if !validCampaignSubUsecase(su) {
			invalidSubUsecases = append(invalidSubUsecases, su)
		}
	}

	if invalidUsecase || len(invalidSubUsecases) > 0 || len(mustBeTrue) > 0 {
		var parts []string
		if invalidUsecase {
			parts = append(parts, "--usecase must be one of: "+strings.Join(CampaignUsecases, ", "))
		}
		if len(invalidSubUsecases) > 0 {
			parts = append(parts, "--sub-usecase "+strings.Join(invalidSubUsecases, ", ")+
				" must be one of: "+strings.Join(CampaignSubUsecases, ", "))
		}
		if len(mustBeTrue) > 0 {
			sorted := append([]string(nil), mustBeTrue...)
			sort.Strings(sorted)
			prefixed := make([]string, len(sorted))
			for i, f := range sorted {
				prefixed[i] = "--" + f
			}
			parts = append(parts, strings.Join(prefixed, ", ")+
				" must be true — the API rejects false on these three as \"is required\" rather "+
				"than as an invalid value, so a campaign cannot declare no opt-in, no opt-out, "+
				"and no help support")
		}
		if len(missing) > 0 {
			// Sorted the same way cmdutil.NewMissingFlagsError sorts its own
			// list, so the rendering of the same missing flags does not depend
			// on whether an enum or must-be-true violation also happened to be
			// present.
			sorted := append([]string(nil), missing...)
			sort.Strings(sorted)
			prefixed := make([]string, len(sorted))
			for i, f := range sorted {
				prefixed[i] = "--" + f
			}
			parts = append(parts, "missing required flags: "+strings.Join(prefixed, ", "))
		}
		return "", cmdutil.NewFlagError(strings.Join(parts, "; "))
	}

	if len(missing) > 0 {
		return "", cmdutil.NewMissingFlagsError(missing)
	}

	return helpAdvisory(o), nil
}

// helpAdvisory returns a non-fatal warning when helpMessage or helpKeywords
// is missing, or "" when both are present. See ValidateCampaignCreate's doc
// comment for why this warns instead of blocking.
func helpAdvisory(o CampaignCreateOptions) string {
	var missing []string
	if o.HelpMessage == "" {
		missing = append(missing, "--help-message")
	}
	if o.HelpKeywords == "" {
		missing = append(missing, "--help-keywords")
	}
	if len(missing) == 0 {
		return ""
	}
	return "warning: " + strings.Join(missing, " and ") +
		" not set; TCR may decline a campaign that lacks a HELP response"
}

func validCampaignUsecase(u string) bool {
	for _, v := range CampaignUsecases {
		if v == u {
			return true
		}
	}
	return false
}

func validCampaignSubUsecase(u string) bool {
	for _, v := range CampaignSubUsecases {
		if v == u {
			return true
		}
	}
	return false
}

// campaignCreateBoolFields maps each boolean flag to the JSON key it writes
// and the option's current value. BuildCampaignCreateRequest only writes an
// entry when changed[flag] is true.
func campaignCreateBoolFields(o CampaignCreateOptions) []struct {
	flag  string
	field string
	value bool
} {
	return []struct {
		flag  string
		field string
		value bool
	}{
		{"embedded-link", "embeddedLink", o.EmbeddedLink},
		{"embedded-phone", "embeddedPhone", o.EmbeddedPhone},
		{"terms-and-conditions", "termsAndConditions", o.TermsAndConditions},
		{"number-pool", "numberPool", o.NumberPool},
		{"age-gated", "ageGated", o.AgeGated},
		{"direct-lending", "directLending", o.DirectLending},
		{"subscriber-optin", "subscriberOptin", o.SubscriberOptin},
		{"subscriber-optout", "subscriberOptout", o.SubscriberOptout},
		{"subscriber-help", "subscriberHelp", o.SubscriberHelp},
		{"auto-renewal", "autoRenewal", o.AutoRenewal},
	}
}

// BuildCampaignCreateRequest builds the POST body, omitting anything unset.
// An empty string is omitted rather than sent, same reasoning as
// BuildBrandCreateRequest: absence leaves the field unset, while "" is a
// value the API validates and rejects.
//
// Booleans cannot use that same "empty means unset" trick — false is not an
// empty value, it is the API's own default, and sending it for a flag the
// user never passed is sending a value, not an absence. So booleans are
// keyed on changed instead: a flag is written only when changed[flag] is
// true, and an explicitly-passed --age-gated=false still reaches the body as
// false, exactly as an explicitly-passed --age-gated=true would.
func BuildCampaignCreateRequest(o CampaignCreateOptions, changed map[string]bool) map[string]any {
	body := map[string]any{}
	for _, p := range [][2]string{
		{"brandId", o.BrandID},
		{"campaignName", o.CampaignName},
		{"usecase", o.Usecase},
		{"description", o.Description},
		{"sample1", o.Sample1},
		{"sample2", o.Sample2},
		{"sample3", o.Sample3},
		{"sample4", o.Sample4},
		{"sample5", o.Sample5},
		{"messageFlow", o.MessageFlow},
		{"helpMessage", o.HelpMessage},
		{"helpKeywords", o.HelpKeywords},
		{"optinMessage", o.OptinMessage},
		{"optinKeywords", o.OptinKeywords},
		{"optoutMessage", o.OptoutMessage},
		{"optoutKeywords", o.OptoutKeywords},
		{"privacyPolicyLink", o.PrivacyPolicyLink},
		{"termsAndConditionsLink", o.TermsAndConditionsLink},
		{"embeddedLinkSample", o.EmbeddedLinkSample},
	} {
		if p[1] != "" {
			body[p[0]] = p[1]
		}
	}

	if len(o.SubUsecases) > 0 {
		body["subUsecases"] = o.SubUsecases
	}

	for _, bf := range campaignCreateBoolFields(o) {
		if changed[bf.flag] {
			body[bf.field] = bf.value
		}
	}

	return body
}

// BuildCampaignSyncRequest re-pulls or revets an existing campaign from TCR.
// It is the same POST /campaigns endpoint as create; a body carrying only
// campaignId, plus campaignName when supplied, is what makes it a sync
// rather than a create — any extra key would turn it into a malformed
// create.
func BuildCampaignSyncRequest(campaignID, campaignName string) map[string]any {
	body := map[string]any{"campaignId": campaignID}
	if campaignName != "" {
		body["campaignName"] = campaignName
	}
	return body
}
