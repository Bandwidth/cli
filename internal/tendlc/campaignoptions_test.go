package tendlc

import (
	"errors"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// completeValid returns options satisfying tier 1 (the five unconditional
// required fields) plus both help fields, so each test can knock out exactly
// one thing. It leaves the tier-2 subscriber booleans at their zero value
// (false) — combined with allTierTwoChanged, that is now DELIBERATELY an
// invalid combination (see TestValidateCampaignCreateRejectsAllThreeExplicitFalse):
// production accepts only true for all three, so completeValid alone is a
// tier-1/help fixture, not a fully-valid one. Tests that need a fully-valid
// payload use withAllSubscriberFieldsValid on top of this.
func completeValid() CampaignCreateOptions {
	return CampaignCreateOptions{
		BrandID:      "BEXMPL1",
		CampaignName: "Acme Notifications",
		Usecase:      "ACCOUNT_NOTIFICATION",
		Description:  "Sends account notifications to opted-in subscribers about their account status.",
		Sample1:      "Your account balance is now available. Reply STOP to opt out.",
		MessageFlow:  "Customer opts in via web form; campaign sends account notifications only.",
		HelpMessage:  "For help, reply HELP or contact support@acme.example.",
		HelpKeywords: "HELP,INFO",
	}
}

// allTierTwoChanged marks the three tier-2 booleans as explicitly passed —
// required for ValidateCampaignCreate to treat them as attestations at all,
// rather than as unset. It does not by itself make a payload valid: the
// values still have to be true (see withAllSubscriberFieldsValid) or the
// mustBeTrue violation fires instead of "missing".
func allTierTwoChanged() map[string]bool {
	return map[string]bool{
		"subscriber-optin":  true,
		"subscriber-optout": true,
		"subscriber-help":   true,
	}
}

// withAllSubscriberFieldsValid returns o with all three tier-2 booleans set
// to true and their tier-3/4 dependents filled in — the only combination of
// these fields production actually accepts. Tests that are not specifically
// about tiers 2-4 use this as their base so they exercise a payload the API
// would accept, not one this validator's earlier version wrongly let through.
func withAllSubscriberFieldsValid(o CampaignCreateOptions) CampaignCreateOptions {
	o.SubscriberOptin = true
	o.OptinMessage = "Text JOIN to opt in."
	o.OptinKeywords = "JOIN"
	o.SubscriberOptout = true
	o.OptoutMessage = "You have been unsubscribed."
	o.OptoutKeywords = "STOP"
	o.SubscriberHelp = true
	return o
}

func TestValidateCampaignCreateAcceptsCompleteOptions(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	advisory, err := ValidateCampaignCreate(o, allTierTwoChanged())
	if err != nil {
		t.Fatalf("want valid, got %v", err)
	}
	if advisory != "" {
		t.Errorf("want no advisory when help fields are set, got %q", advisory)
	}
}

// The API reports every violation at once; so must the CLI. Reporting one
// missing flag per invocation turns a single fix into a multi-round-trip
// slog — tier 1 alone would be five round trips, and tier 2 three more.
func TestValidateCampaignCreateReportsAllMissingFieldsAtOnce(t *testing.T) {
	_, err := ValidateCampaignCreate(CampaignCreateOptions{}, map[string]bool{})
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	var fe *cmdutil.FlagError
	if !errors.As(err, &fe) {
		t.Fatalf("want a FlagError (exit 6), got %T", err)
	}
	for _, want := range []string{
		"--brand-id", "--usecase", "--description", "--sample1", "--message-flow",
		"--subscriber-optin", "--subscriber-optout", "--subscriber-help",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %s: %s", want, err.Error())
		}
	}
	if strings.Contains(err.Error(), "must be true") {
		t.Errorf("unset tier-2 flags must report as missing, not must-be-true: %s", err.Error())
	}
}

// Tier 2, isolated: tier 1 and the help fields are satisfied, but none of
// the three subscriber booleans were passed. All three must be named in one
// error — this is the case the coordinator's live-verification finding was
// built on, and the one broken/restored below to prove it has teeth.
func TestValidateCampaignCreateReportsAllMissingTierTwoFlagsAtOnce(t *testing.T) {
	_, err := ValidateCampaignCreate(completeValid(), map[string]bool{})
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	var fe *cmdutil.FlagError
	if !errors.As(err, &fe) {
		t.Fatalf("want a FlagError (exit 6), got %T", err)
	}
	for _, want := range []string{"--subscriber-optin", "--subscriber-optout", "--subscriber-help"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %s: %s", want, err.Error())
		}
	}
	// An UNSET flag is a "missing" violation, not the "must be true" one —
	// they are different mistakes (see ValidateCampaignCreateRejectsAllThreeExplicitFalse
	// for the other one) and must produce different messages.
	if !strings.Contains(err.Error(), "missing required flags") {
		t.Errorf("unset tier-2 flags should report as missing, got: %s", err.Error())
	}
	if strings.Contains(err.Error(), "must be true") {
		t.Errorf("unset tier-2 flags must not trigger the must-be-true message, got: %s", err.Error())
	}
}

// helpMessage/helpKeywords are spec-required but production accepts a create
// omitting both. They must not appear in the missing-flags error.
func TestValidateCampaignCreateDoesNotRequireHelpFields(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	o.HelpMessage, o.HelpKeywords = "", ""
	_, err := ValidateCampaignCreate(o, allTierTwoChanged())
	if err != nil {
		t.Fatalf("want valid without help fields, got %v", err)
	}
}

// Regression test: an invalid usecase must not suppress missing-flag
// violations — from tier 1 or tier 2. The previous PR shipped this
// short-circuit bug twice on the brand side; the campaign validator must not
// repeat it, at any tier.
func TestValidateCampaignCreateAggregatesInvalidUsecaseWithMissingFlags(t *testing.T) {
	o := CampaignCreateOptions{Usecase: "NOT_A_REAL_USECASE"}
	_, err := ValidateCampaignCreate(o, map[string]bool{})
	if err == nil {
		t.Fatal("want an error for invalid usecase and missing fields")
	}
	// Must mention the invalid usecase and list valid options.
	if !strings.Contains(err.Error(), "ACCOUNT_NOTIFICATION") {
		t.Errorf("error should list valid usecases, got: %s", err.Error())
	}
	// Must also mention the other missing required fields at every tier, not
	// just usecase.
	for _, want := range []string{
		"--brand-id", "--description", "--sample1", "--message-flow",
		"--subscriber-optin", "--subscriber-optout", "--subscriber-help",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %s: %s", want, err.Error())
		}
	}
}

func TestValidateCampaignCreateAcceptsValidUsecase(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	o.Usecase = "2FA"
	if _, err := ValidateCampaignCreate(o, allTierTwoChanged()); err != nil {
		t.Fatalf("want valid usecase accepted, got %v", err)
	}
}

func TestValidateCampaignCreateRejectsUnknownSubUsecase(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	o.SubUsecases = []string{"MARKETING", "NOT_A_REAL_SUBUSECASE"}
	_, err := ValidateCampaignCreate(o, allTierTwoChanged())
	if err == nil {
		t.Fatal("want an error for invalid sub-usecase")
	}
	if !strings.Contains(err.Error(), "NOT_A_REAL_SUBUSECASE") {
		t.Errorf("error should name the invalid sub-usecase, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "MARKETING") {
		t.Errorf("error should list valid sub-usecases, got: %s", err.Error())
	}
}

func TestValidateCampaignCreateAcceptsValidSubUsecases(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	o.SubUsecases = []string{"MARKETING", "CUSTOMER_CARE"}
	if _, err := ValidateCampaignCreate(o, allTierTwoChanged()); err != nil {
		t.Fatalf("want valid sub-usecases accepted, got %v", err)
	}
}

// The spec describes per-usecase count limits on sub-usecases (e.g. 1-5 for
// Low Volume Mixed, 2-5 for Mixed), but ValidateCampaignCreate deliberately
// does not enforce them — unverified against production, per its doc
// comment. This pins that today's validator only checks membership, not
// count, so a future change that starts enforcing an unverified count rule
// (and silently rejects requests production accepts) shows up as a test
// change, not a surprise.
func TestValidateCampaignCreateDoesNotEnforceSubUsecaseCounts(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	o.SubUsecases = []string{
		"2FA", "ACCOUNT_NOTIFICATION", "CUSTOMER_CARE", "DELIVERY_NOTIFICATION",
		"FRAUD_ALERT", "HIGHER_EDUCATION", "MARKETING", "POLLING_VOTING",
		"PUBLIC_SERVICE_ANNOUNCEMENT", "SECURITY_ALERT",
	}
	if _, err := ValidateCampaignCreate(o, allTierTwoChanged()); err != nil {
		t.Fatalf("want all 10 valid sub-usecases accepted regardless of count, got %v", err)
	}
}

// Tier 3: --subscriber-optin passed as true requires both --optin-message
// and --optin-keywords, aggregated into one error naming both. optout and
// help start from a fully-valid base so the only violation in play is optin's.
func TestValidateCampaignCreateRequiresOptinFieldsWhenOptinTrue(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	o.OptinMessage, o.OptinKeywords = "", ""
	_, err := ValidateCampaignCreate(o, allTierTwoChanged())
	if err == nil {
		t.Fatal("want an error when subscriber-optin is true without optin message/keywords")
	}
	for _, want := range []string{"--optin-message", "--optin-keywords"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %s: %s", want, err.Error())
		}
	}
}

// Tier 4: the same shape as tier 3, for --subscriber-optout.
func TestValidateCampaignCreateRequiresOptoutFieldsWhenOptoutTrue(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	o.OptoutMessage, o.OptoutKeywords = "", ""
	_, err := ValidateCampaignCreate(o, allTierTwoChanged())
	if err == nil {
		t.Fatal("want an error when subscriber-optout is true without optout message/keywords")
	}
	for _, want := range []string{"--optout-message", "--optout-keywords"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %s: %s", want, err.Error())
		}
	}
}

// A fully-valid payload exercising every tier — both subscriberOptin and
// subscriberOptout true, with their conditional fields supplied — passes.
func TestValidateCampaignCreateAcceptsFullyValidFourTierPayload(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	if _, err := ValidateCampaignCreate(o, allTierTwoChanged()); err != nil {
		t.Fatalf("want a fully-valid four-tier payload accepted, got %v", err)
	}
}

// The exact case the coordinator's live probe found: --subscriber-optin=false
// --subscriber-optout=false --subscriber-help=false all pass tier 2's
// presence check (every flag was explicitly passed) and previously reached
// this validator's "valid" branch — then 400'd against production with
// "is required" on all three POINTERs, because the API accepts only true
// for these three. All three must now be named in one aggregated error, with
// a message distinct from "missing". completeValid() also leaves optin/optout's
// tier-3/4 fields unset, and a rejected false's only path forward is true —
// which would then demand those fields too — so this error also aggregates
// them in now rather than waiting for a second submission to reveal them
// (see TestValidateCampaignCreateAggregatesTierThreeAndFourWithFalseAttestation
// for that half of the fix in isolation).
func TestValidateCampaignCreateRejectsAllThreeExplicitFalse(t *testing.T) {
	_, err := ValidateCampaignCreate(completeValid(), allTierTwoChanged())
	if err == nil {
		t.Fatal("want an error: production rejects false on all three tier-2 attestations")
	}
	for _, want := range []string{
		"--subscriber-optin", "--subscriber-optout", "--subscriber-help", "must be true",
		"missing required flags", "--optin-message", "--optin-keywords", "--optout-message", "--optout-keywords",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %s", want, err.Error())
		}
	}
}

// F4: an explicit false on subscriber-optin is a REJECTED value, not an
// absent one — the only way forward is true, which would then demand
// optin-message/optin-keywords. Suppressing those two fields for a false
// value the same way they are correctly suppressed for an unset flag meant
// fixing "must be true" and then discovering "missing optin fields" took two
// round trips instead of one. Both must now surface together, in one error.
// subscriber-optout is left true and fully valid so this isolates the optin
// half of the fix from the optout half.
func TestValidateCampaignCreateAggregatesTierThreeAndFourWithFalseAttestation(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	o.SubscriberOptin = false
	o.OptinMessage, o.OptinKeywords = "", ""
	_, err := ValidateCampaignCreate(o, allTierTwoChanged())
	if err == nil {
		t.Fatal("want an error for explicit subscriber-optin=false with no optin message/keywords")
	}
	for _, want := range []string{"--subscriber-optin", "must be true", "--optin-message", "--optin-keywords"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %s", want, err.Error())
		}
	}
	if strings.Contains(err.Error(), "--optout-message") || strings.Contains(err.Error(), "--optout-keywords") {
		t.Errorf("optout's tier-4 fields must not be demanded when optout is true and valid, got: %s", err.Error())
	}
}

// Each of the three tier-2 booleans, passed individually as false against an
// otherwise fully-valid payload, is rejected on its own with the
// must-be-true message — not folded into a generic missing-flags error.
func TestValidateCampaignCreateRejectsEachExplicitFalseTierTwoBoolean(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*CampaignCreateOptions)
		wantFlag string
	}{
		{"subscriber-optin", func(o *CampaignCreateOptions) { o.SubscriberOptin = false }, "--subscriber-optin"},
		{"subscriber-optout", func(o *CampaignCreateOptions) { o.SubscriberOptout = false }, "--subscriber-optout"},
		{"subscriber-help", func(o *CampaignCreateOptions) { o.SubscriberHelp = false }, "--subscriber-help"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := withAllSubscriberFieldsValid(completeValid())
			tt.mutate(&o)
			_, err := ValidateCampaignCreate(o, allTierTwoChanged())
			if err == nil {
				t.Fatalf("want an error for explicit %s=false", tt.name)
			}
			var fe *cmdutil.FlagError
			if !errors.As(err, &fe) {
				t.Fatalf("want a FlagError (exit 6), got %T", err)
			}
			if !strings.Contains(err.Error(), tt.wantFlag) {
				t.Errorf("error missing %s: %s", tt.wantFlag, err.Error())
			}
			if !strings.Contains(err.Error(), "must be true") {
				t.Errorf("error should explain the must-be-true constraint, got: %s", err.Error())
			}
		})
	}
}

// A false attestation aggregated together with an unrelated missing flag —
// the point of this validator is that every violation, of every kind, comes
// back in one error rather than in successive round trips.
func TestValidateCampaignCreateAggregatesFalseAttestationWithMissingFlag(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	o.BrandID = ""           // an unrelated tier-1 missing flag
	o.SubscriberHelp = false // the false attestation
	_, err := ValidateCampaignCreate(o, allTierTwoChanged())
	if err == nil {
		t.Fatal("want an error aggregating both violations")
	}
	if !strings.Contains(err.Error(), "--brand-id") {
		t.Errorf("error should still name the missing --brand-id, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "--subscriber-help") || !strings.Contains(err.Error(), "must be true") {
		t.Errorf("error should name the false attestation with the must-be-true message, got: %s", err.Error())
	}
}

// The advisory is the mechanism for the "not required but flagged" behavior:
// non-fatal, printed to stderr by the command, never blocking the request.
func TestValidateCampaignCreateAdvisoryFiresWhenHelpFieldsAbsent(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	o.HelpMessage, o.HelpKeywords = "", ""
	advisory, err := ValidateCampaignCreate(o, allTierTwoChanged())
	if err != nil {
		t.Fatalf("want no error (advisory is non-fatal), got %v", err)
	}
	if advisory == "" {
		t.Fatal("want a non-empty advisory when help fields are absent")
	}
	if !strings.Contains(advisory, "--help-message") || !strings.Contains(advisory, "--help-keywords") {
		t.Errorf("advisory should name both missing flags, got: %s", advisory)
	}
}

// A partially-set help pair (one of helpMessage/helpKeywords present, the
// other absent) must still fire the advisory, and must name only the field
// that is actually missing — not the one already supplied.
func TestValidateCampaignCreateAdvisoryFiresForPartialHelpPair(t *testing.T) {
	o := withAllSubscriberFieldsValid(completeValid())
	o.HelpKeywords = ""
	advisory, err := ValidateCampaignCreate(o, allTierTwoChanged())
	if err != nil {
		t.Fatalf("want no error (advisory is non-fatal), got %v", err)
	}
	if advisory == "" {
		t.Fatal("want a non-empty advisory when only one help field is set")
	}
	if !strings.Contains(advisory, "--help-keywords") {
		t.Errorf("advisory should name the missing --help-keywords, got: %s", advisory)
	}
	if strings.Contains(advisory, "--help-message") {
		t.Errorf("advisory must not name --help-message, which was supplied: got %s", advisory)
	}
}

func TestValidateCampaignCreateAdvisoryEmptyWhenHelpFieldsPresent(t *testing.T) {
	advisory, err := ValidateCampaignCreate(withAllSubscriberFieldsValid(completeValid()), allTierTwoChanged())
	if err != nil {
		t.Fatalf("want valid, got %v", err)
	}
	if advisory != "" {
		t.Errorf("want empty advisory, got %q", advisory)
	}
}

func TestBuildCampaignCreateRequestOmitsUnsetOptionalFields(t *testing.T) {
	body := BuildCampaignCreateRequest(completeValid(), map[string]bool{})

	if body["brandId"] != "BEXMPL1" {
		t.Errorf("brandId = %v", body["brandId"])
	}
	if body["messageFlow"] == "" {
		t.Errorf("messageFlow should be set")
	}
	for _, k := range []string{
		"sample2", "sample3", "sample4", "sample5", "optinMessage", "optinKeywords",
		"optoutMessage", "optoutKeywords", "privacyPolicyLink", "termsAndConditionsLink",
		"embeddedLinkSample", "subUsecases",
	} {
		if _, present := body[k]; present {
			t.Errorf("unset optional %q must be omitted, got %v", k, body[k])
		}
	}
}

func TestBuildCampaignCreateRequestOmitsUnsetBooleans(t *testing.T) {
	body := BuildCampaignCreateRequest(completeValid(), map[string]bool{})
	for _, k := range []string{
		"embeddedLink", "embeddedPhone", "termsAndConditions", "numberPool", "ageGated",
		"directLending", "subscriberOptin", "subscriberOptout", "subscriberHelp", "autoRenewal",
	} {
		if _, present := body[k]; present {
			t.Errorf("unset boolean %q must be omitted, got %v", k, body[k])
		}
	}
}

// The highest-value boolean test: an explicitly-passed --age-gated=false
// must reach the body as false, not be omitted as if it were never set. A
// build that infers "unset" from the zero value would fail this.
func TestBuildCampaignCreateRequestIncludesExplicitFalseBoolean(t *testing.T) {
	o := completeValid()
	o.AgeGated = false
	body := BuildCampaignCreateRequest(o, map[string]bool{"age-gated": true})

	v, present := body["ageGated"]
	if !present {
		t.Fatal("explicitly-passed --age-gated=false must be present in the body")
	}
	if v != false {
		t.Errorf("ageGated = %v, want false", v)
	}
}

func TestBuildCampaignCreateRequestIncludesExplicitTrueBoolean(t *testing.T) {
	o := completeValid()
	o.NumberPool = true
	body := BuildCampaignCreateRequest(o, map[string]bool{"number-pool": true})

	if body["numberPool"] != true {
		t.Errorf("numberPool = %v, want true", body["numberPool"])
	}
}

// The build side of the tier-2 fix: a create that explicitly passes all
// three subscriber booleans — including one left false — must carry all
// three in the body. A build that dropped the false one (e.g. by
// accidentally gating on the value instead of on changed) would send TCR
// only two of the three fields it requires and still 400.
func TestBuildCampaignCreateRequestIncludesAllThreeTierTwoBooleansWhenChanged(t *testing.T) {
	o := completeValid()
	o.SubscriberOptin = true
	o.SubscriberOptout = false
	o.SubscriberHelp = true
	body := BuildCampaignCreateRequest(o, allTierTwoChanged())

	for field, want := range map[string]bool{
		"subscriberOptin":  true,
		"subscriberOptout": false,
		"subscriberHelp":   true,
	} {
		v, present := body[field]
		if !present {
			t.Errorf("%s must be present when changed, got body = %v", field, body)
			continue
		}
		if v != want {
			t.Errorf("%s = %v, want %v", field, v, want)
		}
	}
}

func TestBuildCampaignCreateRequestIncludesSetOptionalFields(t *testing.T) {
	o := completeValid()
	o.SubUsecases = []string{"MARKETING"}
	o.OptinMessage = "Text JOIN to opt in."
	body := BuildCampaignCreateRequest(o, map[string]bool{})

	subs, ok := body["subUsecases"].([]string)
	if !ok || len(subs) != 1 || subs[0] != "MARKETING" {
		t.Errorf("subUsecases = %v", body["subUsecases"])
	}
	if body["optinMessage"] != "Text JOIN to opt in." {
		t.Errorf("optinMessage = %v", body["optinMessage"])
	}
}

func TestBuildCampaignSyncRequestWithoutName(t *testing.T) {
	body := BuildCampaignSyncRequest("CEXMPL1", "")
	if len(body) != 1 || body["campaignId"] != "CEXMPL1" {
		t.Errorf("body = %v, want exactly {campaignId: CEXMPL1}", body)
	}
}

func TestBuildCampaignSyncRequestWithName(t *testing.T) {
	body := BuildCampaignSyncRequest("CEXMPL1", "Acme Notifications")
	if len(body) != 2 || body["campaignId"] != "CEXMPL1" || body["campaignName"] != "Acme Notifications" {
		t.Errorf("body = %v, want exactly {campaignId: CEXMPL1, campaignName: Acme Notifications}", body)
	}
}
