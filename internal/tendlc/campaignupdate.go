package tendlc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// campaignReadOnlyFields are stripped before a PUT on a direct campaign.
//
// brandId is here even though the API does not document it as read-only: a
// campaign cannot be moved between brands, so accepting a changed brandId
// would be silently misleading. attMessageClass and subId are undocumented
// fields present on every response and always observed null; echoing them
// back is meaningless. status, vettingStatus, and approvals are server-owned
// state, not input.
//
// Note the absence of "version": unlike customer profiles, and like brands,
// campaigns carry no optimistic-locking token at all. RMW here is racy —
// documented, not solved. Do not add a version check.
var campaignReadOnlyFields = []string{
	"accountId", "bandwidthId", "campaignId", "brandDisplayName", "cspId", "cnpId", "resellerId",
	"imported", "createdDate", "modifiedDate", "status", "vettingStatus", "approvals",
	"attMessageClass", "subId", "customerProfileId", "brandId",
}

// CampaignUpdateFieldFlags are every flag `campaign update` accepts, in CLI
// naming. BuildCampaignUpdateRequest keys its changed map on exactly these
// names. brandId, usecase, and subUsecases are absent: none of the three
// appear in the spec's directUpdateCampaignRequest, and a campaign's brand
// and usecase are not editable after creation.
//
// Only 6 of create's 10 booleans are here; see campaignUpdateBoolFlags for
// why the other 4 are deliberately excluded from this flag surface even
// though the campaign resource itself still carries them.
var CampaignUpdateFieldFlags = []string{
	"campaign-name", "description", "sample1", "sample2", "sample3", "sample4", "sample5",
	"message-flow", "help-message", "help-keywords", "optin-message", "optin-keywords",
	"optout-message", "optout-keywords", "privacy-policy-link", "terms-and-conditions-link",
	"embedded-link-sample",
	"embedded-link", "embedded-phone", "number-pool", "age-gated",
	"direct-lending", "auto-renewal",
}

// campaignUpdateBoolFlags is the subset of CampaignUpdateFieldFlags that are
// boolean-typed rather than string-typed: booleans have no zero value that
// means "unset", so the overlay must consult the changed map for these
// instead of the value's emptiness.
//
// Only 6, not create's 10. This is measured, and it is the one place in the
// whole feature where the published spec turned out right and our own
// assumption (this file's own earlier version, and the task brief that
// preceded it) was wrong: termsAndConditions, subscriberOptin,
// subscriberOptout, and subscriberHelp are NOT editable on update.
//
// Measured directly on a live direct campaign by sending one schema-listed
// boolean and one non-listed boolean in the same PUT:
//
//	field            | in directUpdateCampaignRequest? | sent          | result
//	ageGated         | yes                              | false -> true | applied
//	subscriberOptin  | no                                | true -> false | ignored, stayed true
//
// The API returned 202 and silently discarded the field that is not in the
// update schema. Offering CLI flags for the 4 excluded booleans would ship
// four controls that visibly accept a value and silently do nothing —
// exactly the defect class this PR exists to catch.
//
// The 4 excluded booleans are NOT added to campaignReadOnlyFields: they must
// still ride along unmodified in the read-modify-write body. current already
// carries them from the GET, and this full-replacement PUT would null them
// if they were stripped. The change here is only to the flag surface — the
// CLI no longer claims the caller can change them — not to what survives the
// round trip.
//
// subscriberOptin/subscriberOptout (present as read-only data, not as flags)
// are deliberately lowercase "in"/"out" — measured production naming for
// both the request and response bodies. The spec capitalizes them once, in
// the response schema only, and that is a typo. Do not "fix" the casing and
// do not add a response-to-request renaming layer: production is
// self-consistent, which is exactly what was measured to de-risk this
// component.
var campaignUpdateBoolFlags = map[string]bool{
	"embedded-link": true, "embedded-phone": true,
	"number-pool": true, "age-gated": true, "direct-lending": true,
	"auto-renewal": true,
}

// campaignUpdateFlagToField maps a CLI flag name to the JSON key it writes.
var campaignUpdateFlagToField = map[string]string{
	"campaign-name": "campaignName", "description": "description", "sample1": "sample1",
	"sample2": "sample2", "sample3": "sample3", "sample4": "sample4", "sample5": "sample5",
	"message-flow": "messageFlow", "help-message": "helpMessage", "help-keywords": "helpKeywords",
	"optin-message": "optinMessage", "optin-keywords": "optinKeywords",
	"optout-message": "optoutMessage", "optout-keywords": "optoutKeywords",
	"privacy-policy-link": "privacyPolicyLink", "terms-and-conditions-link": "termsAndConditionsLink",
	"embedded-link-sample": "embeddedLinkSample",
	"embedded-link":        "embeddedLink", "embedded-phone": "embeddedPhone",
	"number-pool": "numberPool",
	"age-gated":   "ageGated", "direct-lending": "directLending",
	"auto-renewal": "autoRenewal",
}

// CampaignUpdateOptions is the flag surface of `band tendlc campaign update`.
// Whether a field was explicitly set is tracked separately in the changed
// map, because an empty string, and false, are both legitimate values (clear
// a string field; set a boolean to false) rather than "unset".
type CampaignUpdateOptions struct {
	CampaignName           string
	Description            string
	Sample1                string
	Sample2                string
	Sample3                string
	Sample4                string
	Sample5                string
	MessageFlow            string
	HelpMessage            string
	HelpKeywords           string
	OptinMessage           string
	OptinKeywords          string
	OptoutMessage          string
	OptoutKeywords         string
	PrivacyPolicyLink      string
	TermsAndConditionsLink string
	EmbeddedLinkSample     string

	EmbeddedLink  bool
	EmbeddedPhone bool
	NumberPool    bool
	AgeGated      bool
	DirectLending bool
	AutoRenewal   bool
}

// value returns the string option value for a CLI flag name. Boolean flags
// are not handled here; see boolValue.
func (o CampaignUpdateOptions) value(flag string) string {
	switch flag {
	case "campaign-name":
		return o.CampaignName
	case "description":
		return o.Description
	case "sample1":
		return o.Sample1
	case "sample2":
		return o.Sample2
	case "sample3":
		return o.Sample3
	case "sample4":
		return o.Sample4
	case "sample5":
		return o.Sample5
	case "message-flow":
		return o.MessageFlow
	case "help-message":
		return o.HelpMessage
	case "help-keywords":
		return o.HelpKeywords
	case "optin-message":
		return o.OptinMessage
	case "optin-keywords":
		return o.OptinKeywords
	case "optout-message":
		return o.OptoutMessage
	case "optout-keywords":
		return o.OptoutKeywords
	case "privacy-policy-link":
		return o.PrivacyPolicyLink
	case "terms-and-conditions-link":
		return o.TermsAndConditionsLink
	case "embedded-link-sample":
		return o.EmbeddedLinkSample
	}
	return ""
}

// boolValue returns the boolean option value for a CLI flag name. Only
// meaningful for flags in campaignUpdateBoolFlags.
func (o CampaignUpdateOptions) boolValue(flag string) bool {
	switch flag {
	case "embedded-link":
		return o.EmbeddedLink
	case "embedded-phone":
		return o.EmbeddedPhone
	case "number-pool":
		return o.NumberPool
	case "age-gated":
		return o.AgeGated
	case "direct-lending":
		return o.DirectLending
	case "auto-renewal":
		return o.AutoRenewal
	}
	return false
}

// BuildCampaignUpdateRequest produces a PUT body from the campaign the API
// just returned, branching on that campaign's own "imported" field.
//
// For imported: false (a direct campaign), PUT is a full replacement — the
// same reasoning as BuildBrandUpdateRequest. The body starts as a deep copy
// of current, read-only fields are removed, only explicitly-changed flags
// are overlaid, and the completed body is validated before being returned.
//
// For imported: true, production was measured and the result is the reverse
// of what the design assumed: a PUT with an empty body against an imported
// campaign returned 202, not 400, and left campaignName, description, and
// sample1 unchanged. It is NOT a full replacement there, and it does not
// require campaignName despite the spec's importUpdateCampaignRequest
// marking it required. So the imported arm sends exactly
// {"campaignName": <value>} and nothing else, and only when campaign-name
// was itself explicitly changed. If the caller explicitly changed any other
// flag, that flag would be silently dropped by this branch — the worst
// available outcome — so it is rejected instead with a FlagError naming
// every ignored flag. If campaign-name was not changed (and nothing else
// was either, since that case is already rejected above), there is nothing
// to send: emitting {"campaignName": o.CampaignName} unconditionally would
// send an empty string and actively clear the name, which is the opposite
// of "leave it unchanged."
//
// If "imported" is absent from current, or is present with a non-bool value,
// this returns an error rather than guessing which arm applies.
func BuildCampaignUpdateRequest(current map[string]any, o CampaignUpdateOptions, changed map[string]bool) (map[string]any, error) {
	if current == nil {
		return nil, fmt.Errorf("no current resource to update from")
	}

	imported, ok := current["imported"].(bool)
	if !ok {
		return nil, fmt.Errorf(`campaign response has no usable "imported" field (got %#v); cannot determine which update contract applies`, current["imported"])
	}

	if imported {
		if rejected := ImportedCampaignRejectedFlags(changed); len(rejected) > 0 {
			return nil, cmdutil.NewFlagError("imported campaigns only accept --campaign-name; these flags are ignored and were rejected instead: --" +
				strings.Join(rejected, ", --"))
		}
		// campaign-name must actually have been changed. Unlike the direct
		// branch, an empty changed map is NOT a safe no-op here: emitting
		// {"campaignName": o.CampaignName} unconditionally would send an
		// empty-string name and actively clear it, the opposite of "leave
		// unchanged." With nothing rejected and nothing to apply, there is
		// nothing to send.
		if !changed["campaign-name"] {
			return nil, cmdutil.NewFlagError("imported campaigns only accept --campaign-name; no flags were changed, so there is nothing to update")
		}
		return map[string]any{"campaignName": o.CampaignName}, nil
	}

	body := deepCopyCampaignMap(current)
	for _, ro := range campaignReadOnlyFields {
		delete(body, ro)
	}

	for _, flag := range CampaignUpdateFieldFlags {
		if !changed[flag] {
			continue
		}
		field := campaignUpdateFlagToField[flag]
		if campaignUpdateBoolFlags[flag] {
			// An explicitly-passed --age-gated=false is a real value, not an
			// absence, and must reach the wire as false.
			body[field] = o.boolValue(flag)
			continue
		}
		if v := o.value(flag); v != "" {
			body[field] = v
		} else {
			// An explicitly empty value clears the field; it must go over the
			// wire as JSON null, matching BuildBrandUpdateRequest's reasoning.
			body[field] = nil
		}
	}

	if err := ValidateCampaignUpdate(body); err != nil {
		return nil, err
	}
	return body, nil
}

// ValidateCampaignUpdate checks the fully overlaid PUT body for a direct
// campaign, not the options struct — a struct that looks fine alone can
// still combine with the read campaign into something the API rejects.
//
// Only description, messageFlow, and sample1 are checked — measured against
// production, not the schema. The spec's directUpdateCampaignRequest also
// marks helpMessage and helpKeywords required; production does not enforce
// them, so they are not checked here (same asymmetry as
// ValidateCampaignCreate). Every violation is aggregated into one error; this
// does not early-return, matching the API's own behavior of reporting every
// violation in one 400.
func ValidateCampaignUpdate(body map[string]any) error {
	required := [][2]string{
		{"description", "description"},
		{"message-flow", "messageFlow"},
		{"sample1", "sample1"},
	}
	var cleared []string
	for _, p := range required {
		s, ok := body[p[1]].(string)
		if !ok || s == "" {
			cleared = append(cleared, p[0])
		}
	}
	if len(cleared) == 0 {
		return nil
	}
	sort.Strings(cleared)
	return cmdutil.NewFlagError("these fields are required and cannot be cleared: --" +
		strings.Join(cleared, ", --"))
}

// ImportedCampaignRejectedFlags returns the flags the caller explicitly
// changed other than campaign-name, sorted. An imported campaign accepts
// only a campaign-name change; any other explicitly-changed flag would be
// silently discarded by that branch of BuildCampaignUpdateRequest, so the
// caller uses this to name exactly what it is refusing to apply.
func ImportedCampaignRejectedFlags(changed map[string]bool) []string {
	var out []string
	for flag, isChanged := range changed {
		if !isChanged || flag == "campaign-name" {
			continue
		}
		out = append(out, flag)
	}
	sort.Strings(out)
	return out
}

// campaignFlagReachableFields is every JSON body key any `campaign update`
// flag can write — i.e. every value in campaignUpdateFlagToField. UpdateCampaign
// unions the result with campaignNeverDropFields and passes the whole set to
// putReplaceWithReadOnlyRetry as the fields the retry must never drop: a
// field the CLI models at all is mutable customer data (see
// putReplaceWithReadOnlyRetry's INVARIANT), so it is never eligible to be
// silently stripped and re-sent — regardless of whether the caller's most
// recent invocation happened to touch it.
func campaignFlagReachableFields() map[string]bool {
	out := make(map[string]bool, len(campaignUpdateFlagToField))
	for _, field := range campaignUpdateFlagToField {
		out[field] = true
	}
	return out
}

// campaignNeverDropFields are response fields that ride along unmodified in
// the read-modify-write body without ever being reachable by any update
// flag: termsAndConditions, subscriberOptin, subscriberOptout, and
// subscriberHelp — the same 4 booleans campaignUpdateBoolFlags's doc comment
// documents as measured NOT editable on update (as opposed to fields in
// campaignReadOnlyFields, which are stripped before every PUT because they
// are genuinely server-owned).
//
// campaignFlagReachableFields alone cannot protect these: because no update
// flag ever writes them, they never appear in campaignUpdateFlagToField's
// values, so a guard keyed only on "is this field reachable by some flag"
// would leave them exactly as droppable as an unmodeled field. But they are
// not read-only filler either — measured against production, a direct
// campaign holding false for these returns 400 "is required" naming exactly
// these pointers (the same shape ValidateCampaignCreate documents for
// create; see campaignoptions.go). That reads exactly like the shape this
// retry exists to handle — a 400 naming a field present in body — so
// without this explicit list, the retry would strip all three compliance
// attestations off an update that has nothing to do with them (e.g. a plain
// --description change) and report success. This list is therefore unioned
// into the retry's neverDrop set unconditionally by UpdateCampaign, not
// merely when reachable by a flag.
var campaignNeverDropFields = map[string]bool{
	"termsAndConditions": true,
	"subscriberOptin":    true,
	"subscriberOptout":   true,
	"subscriberHelp":     true,
}

// deepCopyCampaignMap copies m so the result shares no mutable structure with
// it. current is read from an api.Envelope the caller may reuse, so a
// shallow copy would leave nested values aliased between the outgoing body
// and the caller's data.
//
// This is this package's own helper, not a reuse of brandupdate.go's
// deepCopyBrandMap: a shared helper is a refactor worth doing once a third
// consumer needs it, not before.
func deepCopyCampaignMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyCampaignValue(v)
	}
	return out
}

func deepCopyCampaignValue(v any) any {
	switch vv := v.(type) {
	case map[string]any:
		return deepCopyCampaignMap(vv)
	case []any:
		out := make([]any, len(vv))
		for i, e := range vv {
			out[i] = deepCopyCampaignValue(e)
		}
		return out
	default:
		return v
	}
}
