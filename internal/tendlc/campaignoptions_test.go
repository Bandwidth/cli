package tendlc

import (
	"errors"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// completeValid returns options satisfying every required field plus both
// help fields, so each test can knock out exactly one thing.
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

func TestValidateCampaignCreateAcceptsCompleteOptions(t *testing.T) {
	advisory, err := ValidateCampaignCreate(completeValid())
	if err != nil {
		t.Fatalf("want valid, got %v", err)
	}
	if advisory != "" {
		t.Errorf("want no advisory when help fields are set, got %q", advisory)
	}
}

// The API reports every violation at once; so must the CLI. Reporting one
// missing flag per invocation turns a single fix into five round trips.
func TestValidateCampaignCreateReportsAllMissingFieldsAtOnce(t *testing.T) {
	_, err := ValidateCampaignCreate(CampaignCreateOptions{})
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	var fe *cmdutil.FlagError
	if !errors.As(err, &fe) {
		t.Fatalf("want a FlagError (exit 6), got %T", err)
	}
	for _, want := range []string{
		"--brand-id", "--usecase", "--description", "--sample1", "--message-flow",
	} {
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
	_, err := ValidateCampaignCreate(o)
	if err != nil {
		t.Fatalf("want valid without help fields, got %v", err)
	}
}

// Regression test: an invalid usecase must not suppress missing-flag
// violations. The previous PR shipped this short-circuit bug twice on the
// brand side; the campaign validator must not repeat it.
func TestValidateCampaignCreateAggregatesInvalidUsecaseWithMissingFlags(t *testing.T) {
	o := CampaignCreateOptions{Usecase: "NOT_A_REAL_USECASE"}
	_, err := ValidateCampaignCreate(o)
	if err == nil {
		t.Fatal("want an error for invalid usecase and missing fields")
	}
	// Must mention the invalid usecase and list valid options.
	if !strings.Contains(err.Error(), "ACCOUNT_NOTIFICATION") {
		t.Errorf("error should list valid usecases, got: %s", err.Error())
	}
	// Must also mention the other missing required fields, not just usecase.
	for _, want := range []string{"--brand-id", "--description", "--sample1", "--message-flow"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %s: %s", want, err.Error())
		}
	}
}

func TestValidateCampaignCreateAcceptsValidUsecase(t *testing.T) {
	o := completeValid()
	o.Usecase = "2FA"
	if _, err := ValidateCampaignCreate(o); err != nil {
		t.Fatalf("want valid usecase accepted, got %v", err)
	}
}

func TestValidateCampaignCreateRejectsUnknownSubUsecase(t *testing.T) {
	o := completeValid()
	o.SubUsecases = []string{"MARKETING", "NOT_A_REAL_SUBUSECASE"}
	_, err := ValidateCampaignCreate(o)
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
	if _, err := ValidateCampaignCreate(o); err != nil {
		t.Fatalf("want valid sub-usecases accepted, got %v", err)
	}
}

// The advisory is the mechanism for the "not required but flagged" behavior:
// non-fatal, printed to stderr by the command, never blocking the request.
func TestValidateCampaignCreateAdvisoryFiresWhenHelpFieldsAbsent(t *testing.T) {
	o := completeValid()
	o.HelpMessage, o.HelpKeywords = "", ""
	advisory, err := ValidateCampaignCreate(o)
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

func TestValidateCampaignCreateAdvisoryEmptyWhenHelpFieldsPresent(t *testing.T) {
	advisory, err := ValidateCampaignCreate(completeValid())
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
