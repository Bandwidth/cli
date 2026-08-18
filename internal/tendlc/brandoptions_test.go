package tendlc

import (
	"errors"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// commonValid returns options satisfying every field required for all brand
// types, so each test can knock out exactly one thing.
func commonValid() BrandCreateOptions {
	return BrandCreateOptions{
		CustomerProfileID:       "2H6qSHb8yLCm76Dw7TAA9W",
		BrandType:               "PRIVATE_PROFIT",
		DisplayName:             "Acme Corp",
		Street:                  "1000 Bandwidth Way",
		City:                    "Raleigh",
		State:                   "NC",
		PostalCode:              "27606",
		CountryCodeA3:           "USA",
		Phone:                   "+19195551234",
		Email:                   "ops@acme.com",
		CompanyName:             "Acme Corporation Inc",
		Vertical:                "TECHNOLOGY",
		EIN:                     "562242657",
		EINIssuingCountryCodeA3: "USA",
	}
}

func TestValidateBrandCreateAcceptsCompleteOptions(t *testing.T) {
	if err := ValidateBrandCreate(commonValid()); err != nil {
		t.Fatalf("want valid, got %v", err)
	}
}

// The API reports every violation at once; so must the CLI. Reporting one
// missing flag per invocation turns a single fix into nine round trips.
func TestValidateBrandCreateReportsAllMissingCommonFieldsAtOnce(t *testing.T) {
	err := ValidateBrandCreate(BrandCreateOptions{BrandType: "PRIVATE_PROFIT"})
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	var fe *cmdutil.FlagError
	if !errors.As(err, &fe) {
		t.Fatalf("want a FlagError (exit 6), got %T", err)
	}
	for _, want := range []string{
		"--customer-profile-id", "--display-name", "--street", "--city",
		"--state", "--postal-code", "--country-code-a3", "--phone", "--email",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %s: %s", want, err.Error())
		}
	}
}

// There is no --country flag: country is derived server-side from
// countryCodeA3. Measured 2026-08-18 — a create with countryCodeA3 and no
// country succeeds.
func TestValidateBrandCreateDoesNotRequireCountry(t *testing.T) {
	if err := ValidateBrandCreate(commonValid()); err != nil {
		t.Fatalf("want valid without a country field, got %v", err)
	}
	body := BuildBrandCreateRequest(commonValid())
	if _, present := body["country"]; present {
		t.Error("body must not carry a country field; the API derives it")
	}
}

func TestValidateBrandCreatePerTypeRequirements(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*BrandCreateOptions)
		wantFlags []string
		wantOK    bool
	}{
		{
			name: "PRIVATE_PROFIT needs company/vertical/ein",
			mutate: func(o *BrandCreateOptions) {
				o.CompanyName, o.Vertical, o.EIN, o.EINIssuingCountryCodeA3 = "", "", "", ""
			},
			wantFlags: []string{"--company-name", "--vertical", "--ein", "--ein-issuing-country-code-a3"},
		},
		{
			name: "NON_PROFIT has the same extra requirements",
			mutate: func(o *BrandCreateOptions) {
				o.BrandType = "NON_PROFIT"
				o.CompanyName, o.Vertical, o.EIN, o.EINIssuingCountryCodeA3 = "", "", "", ""
			},
			wantFlags: []string{"--company-name", "--vertical", "--ein", "--ein-issuing-country-code-a3"},
		},
		{
			name: "GOVERNMENT requires company-name, vertical, ein, ein-issuing-country-code-a3",
			mutate: func(o *BrandCreateOptions) {
				o.BrandType = "GOVERNMENT"
				o.CompanyName, o.Vertical, o.EIN, o.EINIssuingCountryCodeA3 = "", "", "", ""
			},
			wantFlags: []string{"--company-name", "--vertical", "--ein", "--ein-issuing-country-code-a3"},
		},
		{
			name: "GOVERNMENT does not require website",
			mutate: func(o *BrandCreateOptions) {
				o.BrandType = "GOVERNMENT"
				o.Website = ""
			},
			wantOK: true,
		},
		{
			name: "PUBLIC_PROFIT needs four more schema-optional fields",
			mutate: func(o *BrandCreateOptions) {
				o.BrandType = "PUBLIC_PROFIT"
			},
			wantFlags: []string{"--stock-symbol", "--stock-exchange", "--website", "--business-contact-email"},
		},
		{
			name: "PUBLIC_PROFIT passes once those four are set",
			mutate: func(o *BrandCreateOptions) {
				o.BrandType = "PUBLIC_PROFIT"
				o.StockSymbol, o.StockExchange = "BAND", "NASDAQ"
				o.Website, o.BusinessContactEmail = "https://acme.com", "cfo@acme.com"
			},
			wantOK: true,
		},
		{
			// SOLE_PROPRIETOR is account-gated; its field rules could not be
			// observed on any account available to us. Guessing them is how a
			// validator ends up rejecting what the API would accept, so the CLI
			// checks the common fields and lets the API speak for the rest.
			name: "SOLE_PROPRIETOR skips type-specific validation",
			mutate: func(o *BrandCreateOptions) {
				o.BrandType = "SOLE_PROPRIETOR"
				o.CompanyName, o.Vertical, o.EIN, o.EINIssuingCountryCodeA3 = "", "", "", ""
			},
			wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := commonValid()
			tt.mutate(&o)
			err := ValidateBrandCreate(o)
			if tt.wantOK {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			for _, want := range tt.wantFlags {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error missing %s: %s", want, err.Error())
				}
			}
		})
	}
}

func TestValidateBrandCreateRejectsUnknownBrandType(t *testing.T) {
	o := commonValid()
	o.BrandType = "PRIVATE"
	err := ValidateBrandCreate(o)
	if err == nil {
		t.Fatal("want an error for an unknown brand type")
	}
	if !strings.Contains(err.Error(), "PRIVATE_PROFIT") {
		t.Errorf("error should list the valid types, got: %s", err.Error())
	}
}

func TestValidateBrandCreateRequiresBrandType(t *testing.T) {
	o := commonValid()
	o.BrandType = ""
	err := ValidateBrandCreate(o)
	if err == nil || !strings.Contains(err.Error(), "--brand-type") {
		t.Fatalf("want a --brand-type error, got %v", err)
	}
}

// Regression test: invalid brand type must not suppress other missing-field violations.
// BrandCreateOptions{BrandType: "PRIVATE"} (a typo) should report the invalid type
// AND the missing common fields in a single error.
func TestValidateBrandCreateAggregatesViolationsWithInvalidBrandType(t *testing.T) {
	o := BrandCreateOptions{BrandType: "PRIVATE"}
	err := ValidateBrandCreate(o)
	if err == nil {
		t.Fatal("want an error for invalid brand type and missing fields")
	}
	// Should mention the invalid brand type and list valid options
	if !strings.Contains(err.Error(), "PRIVATE_PROFIT") {
		t.Errorf("error should list valid types, got: %s", err.Error())
	}
	// Should also mention at least some missing common fields
	for _, want := range []string{"--customer-profile-id", "--display-name"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %s: %s", want, err.Error())
		}
	}
}

func TestBuildBrandCreateRequestOmitsUnsetOptionalFields(t *testing.T) {
	body := BuildBrandCreateRequest(commonValid())

	if body["displayName"] != "Acme Corp" {
		t.Errorf("displayName = %v", body["displayName"])
	}
	if body["einIssuingCountryCodeA3"] != "USA" {
		t.Errorf("einIssuingCountryCodeA3 = %v", body["einIssuingCountryCodeA3"])
	}
	// Unset optionals are omitted, not sent as "". An empty string is a value
	// the API validates; absence is not.
	for _, k := range []string{"website", "stockSymbol", "stockExchange", "altBusinessId",
		"altBusinessIdType", "businessContactEmail", "firstName", "lastName",
		"mobilePhone", "ipAddress"} {
		if _, present := body[k]; present {
			t.Errorf("unset optional %q must be omitted, got %v", k, body[k])
		}
	}
}

func TestBuildBrandCreateRequestIncludesSetOptionalFields(t *testing.T) {
	o := commonValid()
	o.Website = "https://acme.com"
	o.MobilePhone = "+19195559999"
	body := BuildBrandCreateRequest(o)

	if body["website"] != "https://acme.com" {
		t.Errorf("website = %v", body["website"])
	}
	if body["mobilePhone"] != "+19195559999" {
		t.Errorf("mobilePhone = %v", body["mobilePhone"])
	}
}

// Refresh reuses POST /brands with a body carrying only brandId. Sending
// anything else would be read as a create.
func TestBuildBrandRefreshRequestSendsOnlyBrandID(t *testing.T) {
	body := BuildBrandRefreshRequest("BGJR2BA")
	if len(body) != 1 || body["brandId"] != "BGJR2BA" {
		t.Errorf("body = %v, want exactly {brandId: BGJR2BA}", body)
	}
}
