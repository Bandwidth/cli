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
// (false) — callers that need tier 2 satisfied must pass allTierTwoChanged
// (or a more specific changed map) alongside it, since tier 2's requirement
// is presence in changed, not the value in this struct.
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

// allTierTwoChanged marks the three tier-2 booleans as explicitly passed,
// the minimum `changed` set that satisfies ValidateCampaignCreate on top of
// completeValid's other fields. Values are read from the options struct
// itself (all false by default in completeValid), so tiers 3/4 do not fire
// unless a test also sets SubscriberOptin/SubscriberOptout to true.
func allTierTwoChanged() map[string]bool {
	return map[string]bool{
		"subscriber-optin":  true,
		"subscriber-optout": true,
		"subscriber-help":   true,
	}
}

func TestValidateCampaignCreateAcceptsCompleteOptions(t *testing.T) {
	advisory, err := ValidateCampaignCreate(completeValid(), allTierTwoChanged())
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
}

// helpMessage/helpKeywords are spec-required but production accepts a create
// omitting both. They must not appear in the missing-flags error.
func TestValidateCampaignCreateDoesNotRequireHelpFields(t *testing.T) {
	o := completeValid()
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
	o := completeValid()
	o.Usecase = "2FA"
	if _, err := ValidateCampaignCreate(o, allTierTwoChanged()); err != nil {
		t.Fatalf("want valid usecase accepted, got %v", err)
	}
}

func TestValidateCampaignCreateRejectsUnknownSubUsecase(t *testing.T) {
	o := completeValid()
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
	o := completeValid()
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
	o := completeValid()
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
// and --optin-keywords, aggregated into one error naming both.
func TestValidateCampaignCreateRequiresOptinFieldsWhenOptinTrue(t *testing.T) {
	o := completeValid()
	o.SubscriberOptin = true
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
	o := completeValid()
	o.SubscriberOptout = true
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
	o := completeValid()
	o.SubscriberOptin = true
	o.OptinMessage = "Text JOIN to opt in."
	o.OptinKeywords = "JOIN"
	o.SubscriberOptout = true
	o.OptoutMessage = "You have been unsubscribed."
	o.OptoutKeywords = "STOP"
	if _, err := ValidateCampaignCreate(o, allTierTwoChanged()); err != nil {
		t.Fatalf("want a fully-valid four-tier payload accepted, got %v", err)
	}
}

// The other highest-value case: --subscriber-optin=false, explicitly
// passed, satisfies tier 2 (the flag was passed) without triggering tier 3
// (its value is false) — optinMessage/optinKeywords must NOT be required.
func TestValidateCampaignCreateExplicitFalseOptinSkipsTierThree(t *testing.T) {
	o := completeValid()
	o.SubscriberOptin = false // explicit, mirrored by changed below
	_, err := ValidateCampaignCreate(o, allTierTwoChanged())
	if err != nil {
		t.Fatalf("want subscriber-optin=false to satisfy tier 2 without demanding tier 3, got %v", err)
	}
}

// The advisory is the mechanism for the "not required but flagged" behavior:
// non-fatal, printed to stderr by the command, never blocking the request.
func TestValidateCampaignCreateAdvisoryFiresWhenHelpFieldsAbsent(t *testing.T) {
	o := completeValid()
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
	o := completeValid()
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
	advisory, err := ValidateCampaignCreate(completeValid(), allTierTwoChanged())
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
