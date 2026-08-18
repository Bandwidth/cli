package tendlc

import (
	"strings"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// BrandTypes are the entity types the API accepts.
var BrandTypes = []string{"PRIVATE_PROFIT", "PUBLIC_PROFIT", "NON_PROFIT", "GOVERNMENT", "SOLE_PROPRIETOR"}

// BrandCreateOptions is the flag surface of `band tendlc brand create`.
//
// Field-to-flag naming is mechanical: CustomerProfileID is --customer-profile-id.
// The one field the API has and this struct does not is `country`, which is
// derived server-side from CountryCodeA3.
type BrandCreateOptions struct {
	CustomerProfileID string
	BrandType         string
	DisplayName       string
	CompanyName       string
	Street            string
	City              string
	State             string
	PostalCode        string
	CountryCodeA3     string
	Phone             string
	Email             string
	Vertical          string
	EIN               string

	EINIssuingCountryCodeA3 string
	Website                 string
	StockSymbol             string
	StockExchange           string
	AltBusinessID           string
	AltBusinessIDType       string
	BusinessContactEmail    string
	FirstName               string
	LastName                string
	MobilePhone             string
	IPAddress               string
}

// commonRequired are required for every brand type, paired as {flag, value}.
func (o BrandCreateOptions) commonRequired() [][2]string {
	return [][2]string{
		{"customer-profile-id", o.CustomerProfileID},
		{"display-name", o.DisplayName},
		{"street", o.Street},
		{"city", o.City},
		{"state", o.State},
		{"postal-code", o.PostalCode},
		{"country-code-a3", o.CountryCodeA3},
		{"phone", o.Phone},
		{"email", o.Email},
	}
}

// registeredEntityRequired are additionally required for every type except
// SOLE_PROPRIETOR, whose rules are unobservable on any account we have.
func (o BrandCreateOptions) registeredEntityRequired() [][2]string {
	return [][2]string{
		{"company-name", o.CompanyName},
		{"vertical", o.Vertical},
		{"ein", o.EIN},
		{"ein-issuing-country-code-a3", o.EINIssuingCountryCodeA3},
	}
}

// publicProfitRequired are the four fields the schema marks optional but that
// production requires for PUBLIC_PROFIT.
func (o BrandCreateOptions) publicProfitRequired() [][2]string {
	return [][2]string{
		{"stock-symbol", o.StockSymbol},
		{"stock-exchange", o.StockExchange},
		{"website", o.Website},
		{"business-contact-email", o.BusinessContactEmail},
	}
}

// ValidateBrandCreate reports every missing required flag in one error, the
// way the API reports every violation in one 400.
//
// The matrix below was derived from the API's own validation errors, not from
// the schema — the schema's required list is wrong in both directions. Only
// the four types we could observe are validated; SOLE_PROPRIETOR is gated at
// the account level, so its field rules fire behind a type check we cannot get
// past, and inventing them would reject requests the API would accept.
func ValidateBrandCreate(o BrandCreateOptions) error {
	var missing []string
	collect := func(pairs [][2]string) {
		for _, p := range pairs {
			if p[1] == "" {
				missing = append(missing, p[0])
			}
		}
	}

	collect(o.commonRequired())

	if o.BrandType == "" {
		missing = append(missing, "brand-type")
	} else if !validBrandType(o.BrandType) {
		return cmdutil.NewFlagError("--brand-type must be one of: " + strings.Join(BrandTypes, ", "))
	}

	switch o.BrandType {
	case "PRIVATE_PROFIT", "NON_PROFIT", "GOVERNMENT":
		collect(o.registeredEntityRequired())
	case "PUBLIC_PROFIT":
		collect(o.registeredEntityRequired())
		collect(o.publicProfitRequired())
	}

	if len(missing) > 0 {
		return cmdutil.NewMissingFlagsError(missing)
	}
	return nil
}

func validBrandType(t string) bool {
	for _, bt := range BrandTypes {
		if bt == t {
			return true
		}
	}
	return false
}

// BuildBrandCreateRequest builds the POST body, omitting anything unset. An
// empty string is omitted rather than sent: absence leaves the field unset,
// while "" is a value the API validates and rejects.
func BuildBrandCreateRequest(o BrandCreateOptions) map[string]any {
	body := map[string]any{}
	for _, p := range [][2]string{
		{"customerProfileId", o.CustomerProfileID},
		{"brandType", o.BrandType},
		{"displayName", o.DisplayName},
		{"companyName", o.CompanyName},
		{"street", o.Street},
		{"city", o.City},
		{"state", o.State},
		{"postalCode", o.PostalCode},
		{"countryCodeA3", o.CountryCodeA3},
		{"phone", o.Phone},
		{"email", o.Email},
		{"vertical", o.Vertical},
		{"ein", o.EIN},
		{"einIssuingCountryCodeA3", o.EINIssuingCountryCodeA3},
		{"website", o.Website},
		{"stockSymbol", o.StockSymbol},
		{"stockExchange", o.StockExchange},
		{"altBusinessId", o.AltBusinessID},
		{"altBusinessIdType", o.AltBusinessIDType},
		{"businessContactEmail", o.BusinessContactEmail},
		{"firstName", o.FirstName},
		{"lastName", o.LastName},
		{"mobilePhone", o.MobilePhone},
		{"ipAddress", o.IPAddress},
	} {
		if p[1] != "" {
			body[p[0]] = p[1]
		}
	}
	return body
}

// BuildBrandRefreshRequest re-pulls an existing brand from TCR. It is the same
// POST /brands endpoint as create; a body carrying only brandId is what makes
// it a refresh, so nothing else may be added here.
func BuildBrandRefreshRequest(brandID string) map[string]any {
	return map[string]any{"brandId": brandID}
}
