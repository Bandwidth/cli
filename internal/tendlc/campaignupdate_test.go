package tendlc

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// liveDirectCampaign mirrors the shape GET /campaigns/{id} actually returns
// for a direct (imported: false) campaign, including read-only and
// undocumented keys plus a field the CLI does not model and nested
// structures the CLI does not model either. Tests overlay against this
// rather than a tidy hand-made map, because the whole point of RMW is
// surviving fields the CLI does not model.
func liveDirectCampaign() map[string]any {
	return map[string]any{
		"accountId":         "1234567",
		"bandwidthId":       "BWCAMP123",
		"campaignId":        "CAMP9WKR2",
		"brandDisplayName":  "Acme Test Co",
		"cspId":             "CSPXYZ",
		"cnpId":             "CNPXYZ",
		"resellerId":        nil,
		"imported":          false,
		"createdDate":       "2026-05-28T20:34:05.480Z",
		"modifiedDate":      "2026-06-17T19:37:16.929Z",
		"status":            "REGISTERED",
		"vettingStatus":     nil,
		"approvals":         []any{map[string]any{"carrierId": "ATT", "status": "APPROVED"}},
		"attMessageClass":   nil,
		"subId":             nil,
		"customerProfileId": "PROFILEXYZ",
		"brandId":           "BRANDXYZ",

		"campaignName":           "Acme Alerts",
		"description":            "Account notifications for Acme customers.",
		"sample1":                "Your appointment is confirmed for tomorrow at 3pm.",
		"sample2":                "Your order has shipped and will arrive in 2 days.",
		"sample3":                "",
		"sample4":                "",
		"sample5":                "",
		"messageFlow":            "Customer opts in via a web form at signup.",
		"helpMessage":            "Reply HELP for help, STOP to opt out.",
		"helpKeywords":           "HELP, INFO",
		"optinMessage":           "You are now opted in to Acme Alerts.",
		"optinKeywords":          "START, YES",
		"optoutMessage":          "You have opted out.",
		"optoutKeywords":         "STOP, CANCEL",
		"privacyPolicyLink":      "https://example.com/privacy",
		"termsAndConditionsLink": "https://example.com/terms",
		"embeddedLinkSample":     "https://example.com/track/abc123",

		"embeddedLink":  false,
		"embeddedPhone": false,
		"numberPool":    true,
		"ageGated":      false,
		"directLending": true,
		"autoRenewal":   false,
		// The 4 booleans below are not in campaignUpdateBoolFlags — measured
		// as not editable on update, see that var's doc comment — but they
		// are real response fields that still ride along in the RMW body.
		// Present here, not stripped, so the losslessness test below catches
		// a regression that starts stripping or nulling them.
		"termsAndConditions": true,
		"subscriberOptin":    true,
		"subscriberOptout":   true,
		"subscriberHelp":     true,

		// A field the CLI does not model at all. It must survive the round trip.
		"someFutureField": "keep me",
		// Nested structures representing future API fields the CLI does not
		// model. Not in campaignReadOnlyFields, so they survive the strip
		// step. deepCopyCampaignValue must recursively copy them, or
		// mutations in body will corrupt current. approvals is also nested
		// but is in the strip list, so it cannot serve this purpose.
		"nestedMetadata": map[string]any{
			"key1": "value1",
			"key2": 42,
		},
		"nestedEntries": []any{
			map[string]any{"id": "entry1", "count": 10},
			map[string]any{"id": "entry2", "count": 20},
		},
	}
}

// liveImportedCampaign is liveDirectCampaign with imported flipped to true —
// the arm where PUT is not a full replacement and only campaignName may be
// changed.
func liveImportedCampaign() map[string]any {
	c := liveDirectCampaign()
	c["imported"] = true
	return c
}

// The defining property: changing one field must not disturb any other,
// including fields the CLI has never heard of. This is the guard against
// silent data loss on a full-replacement PUT.
func TestBuildCampaignUpdateRequestIsLossless(t *testing.T) {
	body, err := BuildCampaignUpdateRequest(liveDirectCampaign(),
		CampaignUpdateOptions{Description: "Updated description text, still meeting the minimum length."},
		map[string]bool{"description": true})
	if err != nil {
		t.Fatalf("BuildCampaignUpdateRequest: %v", err)
	}

	if body["description"] != "Updated description text, still meeting the minimum length." {
		t.Errorf("description = %v, want the updated value", body["description"])
	}
	for k, want := range map[string]any{
		"sample2":         "Your order has shipped and will arrive in 2 days.",
		"optinKeywords":   "START, YES",
		"someFutureField": "keep me",
	} {
		if body[k] != want {
			t.Errorf("%s = %v, want %v (must survive an unrelated change)", k, body[k], want)
		}
	}

	for k, want := range map[string]bool{
		// The 6 flag-settable booleans.
		"embeddedLink":  false,
		"embeddedPhone": false,
		"numberPool":    true,
		"ageGated":      false,
		"directLending": true,
		"autoRenewal":   false,
		// The 4 booleans with no CLI flag at all — must still survive an
		// unrelated update untouched, since they are not stripped.
		"termsAndConditions": true,
		"subscriberOptin":    true,
		"subscriberOptout":   true,
		"subscriberHelp":     true,
	} {
		v, ok := body[k]
		if !ok {
			t.Errorf("%s must be present (unmodeled-boolean loss check), got absent", k)
			continue
		}
		if v != want {
			t.Errorf("%s = %v, want %v (must survive an unrelated change)", k, v, want)
		}
	}
}

func TestBuildCampaignUpdateRequestStripsReadOnlyFields(t *testing.T) {
	body, err := BuildCampaignUpdateRequest(liveDirectCampaign(),
		CampaignUpdateOptions{Description: "Updated description text, still meeting the minimum length."},
		map[string]bool{"description": true})
	if err != nil {
		t.Fatalf("BuildCampaignUpdateRequest: %v", err)
	}
	for _, k := range campaignReadOnlyFields {
		if _, present := body[k]; present {
			t.Errorf("read-only field %q must be stripped, got %v", k, body[k])
		}
	}
}

// Campaigns have no version field. A test asserting one would be asserting a
// customer-profile behavior that does not exist here.
func TestBuildCampaignUpdateRequestDoesNotInventAVersion(t *testing.T) {
	body, err := BuildCampaignUpdateRequest(liveDirectCampaign(),
		CampaignUpdateOptions{Description: "Updated description text, still meeting the minimum length."},
		map[string]bool{"description": true})
	if err != nil {
		t.Fatalf("BuildCampaignUpdateRequest: %v", err)
	}
	if _, present := body["version"]; present {
		t.Error("campaigns carry no version; the body must not add one")
	}
}

// A changed boolean must reach the body as a real bool, not be silently
// coerced or dropped. Presence is asserted via comma-ok, not equality —
// body["ageGated"] == true also passes when the key is entirely absent from
// a nil map read, which would not prove presence.
func TestBuildCampaignUpdateRequestChangedBooleanReachesBody(t *testing.T) {
	body, err := BuildCampaignUpdateRequest(liveDirectCampaign(),
		CampaignUpdateOptions{AgeGated: true},
		map[string]bool{"age-gated": true})
	if err != nil {
		t.Fatalf("BuildCampaignUpdateRequest: %v", err)
	}
	v, ok := body["ageGated"]
	if !ok {
		t.Fatal("ageGated must be present in the body")
	}
	if v != true {
		t.Errorf("ageGated = %v, want true", v)
	}
}

// An explicitly-passed --direct-lending=false is a real value and must
// reach the wire as false, overwriting a current value of true — not be
// mistaken for "unset" and left alone.
func TestBuildCampaignUpdateRequestExplicitFalseBooleanIsPresentAndFalse(t *testing.T) {
	current := liveDirectCampaign()
	if current["directLending"] != true {
		t.Fatal("fixture must start with directLending true for this test to prove anything")
	}
	body, err := BuildCampaignUpdateRequest(current,
		CampaignUpdateOptions{DirectLending: false},
		map[string]bool{"direct-lending": true})
	if err != nil {
		t.Fatalf("BuildCampaignUpdateRequest: %v", err)
	}
	v, ok := body["directLending"]
	if !ok {
		t.Fatal("directLending must be present in the body, not omitted")
	}
	if v != false {
		t.Errorf("directLending = %v, want false", v)
	}
}

// A boolean the caller did not touch must keep the value read from current,
// not be reset to the Go zero value or dropped.
func TestBuildCampaignUpdateRequestUnchangedBooleanKeepsReadValue(t *testing.T) {
	current := liveDirectCampaign()
	if current["numberPool"] != true {
		t.Fatal("fixture must start with numberPool true for this test to prove anything")
	}
	body, err := BuildCampaignUpdateRequest(current,
		CampaignUpdateOptions{Description: "Updated description text, still meeting the minimum length."},
		map[string]bool{"description": true})
	if err != nil {
		t.Fatalf("BuildCampaignUpdateRequest: %v", err)
	}
	v, ok := body["numberPool"]
	if !ok {
		t.Fatal("numberPool must be present in the body")
	}
	if v != true {
		t.Errorf("numberPool = %v, want true (unchanged fields must keep the read value)", v)
	}
}

func TestBuildCampaignUpdateRequestDeepCopiesNestedValues(t *testing.T) {
	current := liveDirectCampaign()
	body, err := BuildCampaignUpdateRequest(current,
		CampaignUpdateOptions{Description: "Updated description text, still meeting the minimum length."},
		map[string]bool{"description": true})
	if err != nil {
		t.Fatalf("BuildCampaignUpdateRequest: %v", err)
	}

	// Mutate the nested map in body and verify current is unaffected. This
	// would fail under a shallow copy because nestedMetadata is not in
	// campaignReadOnlyFields so it survives the strip step. The type
	// assertions are unconditional: a missing or wrongly-typed key must fail
	// the test loudly, not silently skip the mutation and the assertion that
	// depends on it.
	nestedMap, ok := body["nestedMetadata"].(map[string]any)
	if !ok {
		t.Fatalf("body[%q] = %#v (%T), want map[string]any", "nestedMetadata", body["nestedMetadata"], body["nestedMetadata"])
	}
	nestedMap["key1"] = "mutated"

	currentNested, ok := current["nestedMetadata"].(map[string]any)
	if !ok {
		t.Fatalf("current[%q] = %#v (%T), want map[string]any", "nestedMetadata", current["nestedMetadata"], current["nestedMetadata"])
	}
	if currentNested["key1"] != "value1" {
		t.Error("mutating nested map in body changed the caller's nested map")
	}

	// Mutate a nested entry in the nested array and verify current is
	// unaffected. This exercises the []any recursion path.
	nestedArray, ok := body["nestedEntries"].([]any)
	if !ok || len(nestedArray) == 0 {
		t.Fatalf("body[%q] = %#v, want a non-empty []any", "nestedEntries", body["nestedEntries"])
	}
	nestedEntry, ok := nestedArray[0].(map[string]any)
	if !ok {
		t.Fatalf("body[%q][0] = %#v (%T), want map[string]any", "nestedEntries", nestedArray[0], nestedArray[0])
	}
	nestedEntry["count"] = 999

	currentArray, ok := current["nestedEntries"].([]any)
	if !ok || len(currentArray) == 0 {
		t.Fatalf("current[%q] = %#v, want a non-empty []any", "nestedEntries", current["nestedEntries"])
	}
	currentEntry, ok := currentArray[0].(map[string]any)
	if !ok {
		t.Fatalf("current[%q][0] = %#v (%T), want map[string]any", "nestedEntries", currentArray[0], currentArray[0])
	}
	if currentEntry["count"] != 10 {
		t.Error("mutating nested array entry in body changed the caller's nested array")
	}

	// The read map must also be untouched by the strip step.
	if _, present := current["bandwidthId"]; !present {
		t.Error("stripping read-only fields mutated the caller's map")
	}
}

func TestBuildCampaignUpdateRequestRejectsNilCurrent(t *testing.T) {
	if _, err := BuildCampaignUpdateRequest(nil, CampaignUpdateOptions{}, map[string]bool{}); err == nil {
		t.Fatal("want an error for a nil current resource")
	}
}

// Validation runs on the completed body, not the options struct: a struct
// that looks fine alone can still combine with the read campaign into
// something the API rejects. Catching it here makes it a zero-request exit 6
// before any PUT could happen.
func TestBuildCampaignUpdateRequestValidatesCompletedBody(t *testing.T) {
	_, err := BuildCampaignUpdateRequest(liveDirectCampaign(),
		CampaignUpdateOptions{Description: ""},
		map[string]bool{"description": true})
	if err == nil {
		t.Fatal("want an error when a required field is cleared")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error should name the field, got: %s", err.Error())
	}
}

// ValidateCampaignUpdate must aggregate every violation into one error
// instead of early-returning on the first one found.
func TestBuildCampaignUpdateRequestAggregatesClearedFields(t *testing.T) {
	_, err := BuildCampaignUpdateRequest(liveDirectCampaign(),
		CampaignUpdateOptions{Description: "", MessageFlow: ""},
		map[string]bool{"description": true, "message-flow": true})
	if err == nil {
		t.Fatal("want an error when two required fields are cleared")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error should name description, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "message-flow") {
		t.Errorf("error should also name message-flow (must not short-circuit), got: %s", err.Error())
	}
}

func TestValidateCampaignUpdateAggregatesAllThreeRequiredFields(t *testing.T) {
	err := ValidateCampaignUpdate(map[string]any{})
	if err == nil {
		t.Fatal("want an error for an empty body")
	}
	for _, f := range []string{"description", "message-flow", "sample1"} {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("error should name %q, got: %s", f, err.Error())
		}
	}
}

// On an imported campaign the body must be exactly {"campaignName": ...} —
// nothing else, even though current carries dozens of other fields.
func TestBuildCampaignUpdateRequestImportedReturnsOnlyCampaignName(t *testing.T) {
	body, err := BuildCampaignUpdateRequest(liveImportedCampaign(),
		CampaignUpdateOptions{CampaignName: "Renamed Campaign"},
		map[string]bool{"campaign-name": true})
	if err != nil {
		t.Fatalf("BuildCampaignUpdateRequest: %v", err)
	}
	if len(body) != 1 {
		t.Fatalf("body has %d keys, want exactly 1: %#v", len(body), body)
	}
	if body["campaignName"] != "Renamed Campaign" {
		t.Errorf("campaignName = %v, want Renamed Campaign", body["campaignName"])
	}
}

// Silently dropping a flag the caller explicitly passed is the worst
// available outcome for an imported campaign, so any flag other than
// campaign-name must be rejected with a FlagError naming it.
func TestBuildCampaignUpdateRequestImportedRejectsOtherFlags(t *testing.T) {
	_, err := BuildCampaignUpdateRequest(liveImportedCampaign(),
		CampaignUpdateOptions{CampaignName: "Renamed Campaign", Description: "New description text of sufficient length."},
		map[string]bool{"campaign-name": true, "description": true})
	if err == nil {
		t.Fatal("want an error when a flag other than campaign-name is changed on an imported campaign")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error should name description, got: %s", err.Error())
	}
}

func TestBuildCampaignUpdateRequestImportedRejectsMultipleOtherFlags(t *testing.T) {
	_, err := BuildCampaignUpdateRequest(liveImportedCampaign(),
		CampaignUpdateOptions{Description: "New description text of sufficient length.", AgeGated: true},
		map[string]bool{"description": true, "age-gated": true})
	if err == nil {
		t.Fatal("want an error when multiple flags other than campaign-name are changed on an imported campaign")
	}
	for _, f := range []string{"description", "age-gated"} {
		if !strings.Contains(err.Error(), f) {
			t.Errorf("error should name %q, got: %s", f, err.Error())
		}
	}
}

// The imported branch must not send an empty-string campaignName just
// because the caller changed nothing. An empty changed map has nothing to
// apply, so this must be an error rather than a silent name-clearing PUT.
func TestBuildCampaignUpdateRequestImportedErrorsWhenNoFlagsChanged(t *testing.T) {
	_, err := BuildCampaignUpdateRequest(liveImportedCampaign(), CampaignUpdateOptions{}, map[string]bool{})
	if err == nil {
		t.Fatal("want an error when no flags were changed on an imported campaign; there is nothing to send")
	}
}

// campaign-name unchanged but another flag changed must still be rejected
// naming that flag — the rejection does not depend on campaign-name's own
// changed state, only on whether anything OTHER than campaign-name changed.
func TestBuildCampaignUpdateRequestImportedRejectsOtherFlagWithNameUnchanged(t *testing.T) {
	_, err := BuildCampaignUpdateRequest(liveImportedCampaign(),
		CampaignUpdateOptions{Description: "New description text of sufficient length."},
		map[string]bool{"description": true})
	if err == nil {
		t.Fatal("want an error when a flag other than campaign-name changed, even though campaign-name itself did not")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("error should name description, got: %s", err.Error())
	}
}

func TestBuildCampaignUpdateRequestErrorsWhenImportedFieldAbsent(t *testing.T) {
	current := liveDirectCampaign()
	delete(current, "imported")
	_, err := BuildCampaignUpdateRequest(current, CampaignUpdateOptions{Description: "x"}, map[string]bool{"description": true})
	if err == nil {
		t.Fatal("want an error when the campaign response has no imported field")
	}
}

func TestBuildCampaignUpdateRequestErrorsWhenImportedFieldNonBool(t *testing.T) {
	current := liveDirectCampaign()
	current["imported"] = "false" // a string, not a bool
	_, err := BuildCampaignUpdateRequest(current, CampaignUpdateOptions{Description: "x"}, map[string]bool{"description": true})
	if err == nil {
		t.Fatal("want an error when imported is present but not a bool")
	}
}

func TestImportedCampaignRejectedFlags(t *testing.T) {
	got := ImportedCampaignRejectedFlags(map[string]bool{
		"campaign-name": true,
		"description":   true,
		"age-gated":     true,
		"sample1":       false, // explicitly not changed; must not appear
	})
	sort.Strings(got)
	want := []string{"age-gated", "description"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestImportedCampaignRejectedFlagsEmptyWhenOnlyCampaignNameChanged(t *testing.T) {
	got := ImportedCampaignRejectedFlags(map[string]bool{"campaign-name": true})
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}
