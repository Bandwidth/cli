package tendlc

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// liveBrand mirrors the shape GET /brands/{id} actually returns, including
// read-only and undocumented keys. Tests overlay against this rather than a
// tidy hand-made map, because the whole point of RMW is surviving fields the
// CLI does not model.
func liveBrand() map[string]any {
	return map[string]any{
		"accounts": []any{map[string]any{
			"accountId":         "9901287",
			"customerProfileId": "2H6qSHb8yLCm76Dw7TAA9W",
		}},
		"altBusinessId":                    nil,
		"altBusinessIdType":                nil,
		"authenticationStatus":             nil,
		"bandwidthId":                      "WET8JUY8H0",
		"brandId":                          "BGJR2BA",
		"brandIdentityStatus":              "UNVERIFIED",
		"brandRelationship":                "MEDIUM_ACCOUNT",
		"brandType":                        "PRIVATE_PROFIT",
		"businessContactEmail":             "kshah@bandwidth.com",
		"businessContactEmailVerifiedDate": nil,
		"city":                             "Raleigh",
		"companyName":                      "Bandwidth Inc",
		"country":                          "US",
		"countryCodeA3":                    "USA",
		"createdDate":                      "2026-05-28T20:34:05.480Z",
		"displayName":                      "Bandwidth Acceptance Test",
		"ein":                              "562242657",
		"einIssuingCountry":                "US",
		"einIssuingCountryCodeA3":          "USA",
		"email":                            "npatel@bandwidth.com",
		"evpVettingScore":                  nil,
		"imported":                         false,
		"ipAddress":                        nil,
		"modifiedDate":                     "2026-06-17T19:37:16.929Z",
		"phone":                            "+12025551234",
		"postalCode":                       "27606",
		"referenceId":                      "WET8JUY8H0",
		"state":                            "NC",
		"street":                           "1000 Bandwidth Way",
		"universalEin":                     "US_562242657",
		"vertical":                         "PROFESSIONAL",
		"website":                          "https://bandwidth.com",
		// A field the CLI does not model at all. It must survive the round trip.
		"someFutureField": "keep me",
		// These nested structures represent future API fields that the CLI does not
		// model. They are not in brandReadOnlyFields, so they survive the strip step.
		// deepCopyBrandValue must recursively copy them, or mutations in body will
		// corrupt current — this is why deep copy is critical for this component.
		"nestedMetadata": map[string]any{
			"key1": "value1",
			"key2": 42,
		},
		"nestedEntries": []any{
			map[string]any{
				"id":    "entry1",
				"count": 10,
			},
			map[string]any{
				"id":    "entry2",
				"count": 20,
			},
		},
	}
}

// The defining property: changing one field must not disturb any other,
// including fields the CLI has never heard of.
func TestBuildBrandUpdateRequestIsLossless(t *testing.T) {
	body, err := BuildBrandUpdateRequest(liveBrand(),
		BrandUpdateOptions{DisplayName: "Renamed"},
		map[string]bool{"display-name": true})
	if err != nil {
		t.Fatalf("BuildBrandUpdateRequest: %v", err)
	}

	if body["displayName"] != "Renamed" {
		t.Errorf("displayName = %v, want Renamed", body["displayName"])
	}
	for k, want := range map[string]any{
		"companyName":     "Bandwidth Inc",
		"street":          "1000 Bandwidth Way",
		"website":         "https://bandwidth.com",
		"vertical":        "PROFESSIONAL",
		"someFutureField": "keep me",
	} {
		if body[k] != want {
			t.Errorf("%s = %v, want %v (must survive an unrelated change)", k, body[k], want)
		}
	}
}

func TestBuildBrandUpdateRequestStripsReadOnlyFields(t *testing.T) {
	body, err := BuildBrandUpdateRequest(liveBrand(),
		BrandUpdateOptions{DisplayName: "Renamed"},
		map[string]bool{"display-name": true})
	if err != nil {
		t.Fatalf("BuildBrandUpdateRequest: %v", err)
	}
	for _, k := range []string{
		"accounts", "bandwidthId", "brandId", "brandIdentityStatus", "brandRelationship",
		"authenticationStatus", "businessContactEmailVerifiedDate", "createdDate",
		"modifiedDate", "evpVettingScore", "imported", "universalEin", "country",
		"einIssuingCountry", "referenceId",
	} {
		if _, present := body[k]; present {
			t.Errorf("read-only field %q must be stripped, got %v", k, body[k])
		}
	}
}

// Brands have no version field. A test asserting one would be asserting a
// customer-profile behavior that does not exist here.
func TestBuildBrandUpdateRequestDoesNotInventAVersion(t *testing.T) {
	body, err := BuildBrandUpdateRequest(liveBrand(),
		BrandUpdateOptions{DisplayName: "Renamed"},
		map[string]bool{"display-name": true})
	if err != nil {
		t.Fatalf("BuildBrandUpdateRequest: %v", err)
	}
	if _, present := body["version"]; present {
		t.Error("brands carry no version; the body must not add one")
	}
}

// The documented way to clear a field is passing the flag with an empty value.
// That must reach the wire as JSON null — the API rejects "" on at least one
// field and accepts null.
func TestBuildBrandUpdateRequestClearsWithNull(t *testing.T) {
	body, err := BuildBrandUpdateRequest(liveBrand(),
		BrandUpdateOptions{Website: ""},
		map[string]bool{"website": true})
	if err != nil {
		t.Fatalf("BuildBrandUpdateRequest: %v", err)
	}
	v, present := body["website"]
	if !present {
		t.Fatal("website must be present as null, not omitted")
	}
	if v != nil {
		t.Errorf("website = %v, want nil", v)
	}
}

func TestBuildBrandUpdateRequestDeepCopiesNestedValues(t *testing.T) {
	current := liveBrand()
	body, err := BuildBrandUpdateRequest(current,
		BrandUpdateOptions{DisplayName: "Renamed"},
		map[string]bool{"display-name": true})
	if err != nil {
		t.Fatalf("BuildBrandUpdateRequest: %v", err)
	}

	// Mutate the nested map in body and verify current is unaffected.
	// This would fail under a shallow copy because nestedMetadata is not in
	// brandReadOnlyFields so it survives the strip step. The type assertions
	// are unconditional: a missing or wrongly-typed key must fail the test
	// loudly, not silently skip the mutation and the assertion that depends
	// on it.
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

	// Mutate a nested entry in the nested array and verify current is unaffected.
	// This exercises the []any recursion path.
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

func TestBuildBrandUpdateRequestRejectsNilCurrent(t *testing.T) {
	if _, err := BuildBrandUpdateRequest(nil, BrandUpdateOptions{}, map[string]bool{}); err == nil {
		t.Fatal("want an error for a nil current resource")
	}
}

// Validation runs on the completed body, not the options struct: a struct
// that looks fine alone can still combine with the read brand into something
// the API rejects. Catching it here makes it a zero-request exit 6.
func TestBuildBrandUpdateRequestValidatesCompletedBody(t *testing.T) {
	_, err := BuildBrandUpdateRequest(liveBrand(),
		BrandUpdateOptions{DisplayName: ""},
		map[string]bool{"display-name": true})
	if err == nil {
		t.Fatal("want an error when a required field is cleared")
	}
	if !strings.Contains(err.Error(), "display-name") {
		t.Errorf("error should name the flag, got: %s", err.Error())
	}
}

// ValidateBrandCreate rejects a brand-type typo against BrandTypes via
// validBrandType, so the same typo must not slip through on update just
// because the string is non-empty — that asymmetry would turn a local
// exit-6 FlagError into a raw 400 from the API.
func TestBuildBrandUpdateRequestRejectsInvalidBrandType(t *testing.T) {
	_, err := BuildBrandUpdateRequest(liveBrand(),
		BrandUpdateOptions{BrandType: "NOT_A_REAL_TYPE"},
		map[string]bool{"brand-type": true})
	if err == nil {
		t.Fatal("want an error for an invalid --brand-type value")
	}
	if !strings.Contains(err.Error(), "brand-type") {
		t.Errorf("error should name the flag, got: %s", err.Error())
	}
	for _, bt := range BrandTypes {
		if !strings.Contains(err.Error(), bt) {
			t.Errorf("error should list valid brand types (missing %q), got: %s", bt, err.Error())
		}
	}
}

// Task 2 of this plan shipped an early return that discarded already-collected
// violations when brand-type was invalid. ValidateBrandUpdate must aggregate:
// an invalid brand-type value and a cleared required field must both surface
// in the same error, not have one hide the other.
func TestBuildBrandUpdateRequestAggregatesInvalidBrandTypeWithClearedFields(t *testing.T) {
	_, err := BuildBrandUpdateRequest(liveBrand(),
		BrandUpdateOptions{BrandType: "NOT_A_REAL_TYPE", DisplayName: ""},
		map[string]bool{"brand-type": true, "display-name": true})
	if err == nil {
		t.Fatal("want an error for an invalid --brand-type value combined with a cleared required field")
	}
	if !strings.Contains(err.Error(), "brand-type") {
		t.Errorf("error should name brand-type, got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "display-name") {
		t.Errorf("error should also name display-name (must not short-circuit), got: %s", err.Error())
	}
}

func TestIdentityFieldsChanged(t *testing.T) {
	tests := []struct {
		name    string
		current map[string]any
		changed map[string]bool
		want    []string
	}{
		{
			name:    "no identity fields touched",
			current: liveBrand(),
			changed: map[string]bool{"website": true, "street": true},
			want:    nil,
		},
		{
			name:    "company name is an identity field",
			current: liveBrand(),
			changed: map[string]bool{"company-name": true},
			want:    []string{"company-name"},
		},
		{
			name:    "all four identity fields plus mobile phone",
			current: liveBrand(),
			changed: map[string]bool{
				"company-name": true, "brand-type": true, "ein": true,
				"ein-issuing-country-code-a3": true, "mobile-phone": true,
			},
			want: []string{"brand-type", "company-name", "ein", "ein-issuing-country-code-a3", "mobile-phone"},
		},
		{
			// Changing businessContactEmail revokes Auth+ compliance, but only
			// on PUBLIC_PROFIT brands.
			name:    "business contact email counts only on PUBLIC_PROFIT",
			current: liveBrand(), // PRIVATE_PROFIT
			changed: map[string]bool{"business-contact-email": true},
			want:    nil,
		},
		{
			name: "business contact email counts on PUBLIC_PROFIT",
			current: func() map[string]any {
				b := liveBrand()
				b["brandType"] = "PUBLIC_PROFIT"
				return b
			}(),
			changed: map[string]bool{"business-contact-email": true},
			want:    []string{"business-contact-email"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IdentityFieldsChanged(tt.current, tt.changed)
			sort.Strings(got)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
