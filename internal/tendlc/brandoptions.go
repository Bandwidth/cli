package tendlc

import (
	"sort"
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

// registeredEntityRequiredFlags are additionally required for every brand
// type except SOLE_PROPRIETOR, whose rules are unobservable on any account we
// have. Shared with ValidateBrandUpdate (brandupdate.go) so the per-type tier
// is defined exactly once for both create and update.
var registeredEntityRequiredFlags = []string{"company-name", "vertical", "ein", "ein-issuing-country-code-a3"}

// publicProfitRequiredFlags are the four fields the schema marks optional but
// that production requires for PUBLIC_PROFIT, layered on top of
// registeredEntityRequiredFlags. Shared with ValidateBrandUpdate for the same
// reason as registeredEntityRequiredFlags.
var publicProfitRequiredFlags = []string{"stock-symbol", "stock-exchange", "website", "business-contact-email"}

// value returns the option value for a CLI flag name understood by the
// per-type requirement tiers above. Mirrors BrandUpdateOptions.value in
// brandupdate.go.
func (o BrandCreateOptions) value(flag string) string {
	switch flag {
	case "company-name":
		return o.CompanyName
	case "vertical":
		return o.Vertical
	case "ein":
		return o.EIN
	case "ein-issuing-country-code-a3":
		return o.EINIssuingCountryCodeA3
	case "stock-symbol":
		return o.StockSymbol
	case "stock-exchange":
		return o.StockExchange
	case "website":
		return o.Website
	case "business-contact-email":
		return o.BusinessContactEmail
	}
	return ""
}

// registeredEntityRequired are additionally required for every type except
// SOLE_PROPRIETOR, whose rules are unobservable on any account we have.
func (o BrandCreateOptions) registeredEntityRequired() [][2]string {
	pairs := make([][2]string, len(registeredEntityRequiredFlags))
	for i, f := range registeredEntityRequiredFlags {
		pairs[i] = [2]string{f, o.value(f)}
	}
	return pairs
}

// publicProfitRequired are the four fields the schema marks optional but that
// production requires for PUBLIC_PROFIT.
func (o BrandCreateOptions) publicProfitRequired() [][2]string {
	pairs := make([][2]string, len(publicProfitRequiredFlags))
	for i, f := range publicProfitRequiredFlags {
		pairs[i] = [2]string{f, o.value(f)}
	}
	return pairs
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
	var invalidBrandType bool
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
		invalidBrandType = true
	} else {
		// Only collect per-type requirements if BrandType is valid
		switch o.BrandType {
		case "PRIVATE_PROFIT", "NON_PROFIT", "GOVERNMENT":
			collect(o.registeredEntityRequired())
		case "PUBLIC_PROFIT":
			collect(o.registeredEntityRequired())
			collect(o.publicProfitRequired())
		}
	}

	// If brand type is invalid, combine the enum error with any missing flags.
	// Sorted the same way cmdutil.NewMissingFlagsError sorts its own list, so
	// the rendering of the same missing flags doesn't depend on whether the
	// brand type also happened to be invalid.
	if invalidBrandType {
		enumMsg := "--brand-type must be one of: " + strings.Join(BrandTypes, ", ")
		if len(missing) > 0 {
			sorted := append([]string(nil), missing...)
			sort.Strings(sorted)
			prefixedMissing := make([]string, len(sorted))
			for i, f := range sorted {
				prefixedMissing[i] = "--" + f
			}
			return cmdutil.NewFlagError(enumMsg + "; missing required flags: " + strings.Join(prefixedMissing, ", "))
		}
		return cmdutil.NewFlagError(enumMsg)
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
//
// brandID is sent in the BODY, not the path — the one command in this
// package that puts an identifier there instead of in the URL. Measured
// against production: `brand refresh WET8JUY8H0` (a bandwidthId, not a TCR
// brandId) resolved correctly and returned {bandwidthId: WET8JUY8H0, brandId:
// BGJR2BA}, so the "every command accepts either ID" claim holds here too —
// it was previously assumed, not verified, for this one command.
func BuildBrandRefreshRequest(brandID string) map[string]any {
	return map[string]any{"brandId": brandID}
}
