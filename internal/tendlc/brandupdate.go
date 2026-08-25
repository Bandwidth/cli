package tendlc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// brandReadOnlyFields are stripped before a PUT.
//
// Production accepts all of them without complaint — measured by PUTting a
// full 46-key read response back and getting exactly one error, for the field
// deliberately corrupted to force a 400. They are stripped anyway: sending a
// server-assigned value back is meaningless, and a field that is ignored today
// may be honored tomorrow.
//
// The strip list was also confirmed safe against the opposite failure mode:
// a live `brand update --website` on a real brand was diffed key-by-key
// before and after. All 46 keys were present both times, zero were dropped,
// zero were nulled, and only `website` changed — the server regenerates every
// derived/read-only field itself. So stripping this list does not risk
// silently nulling a caller-supplied field on this full-replacement PUT.
//
// country and einIssuingCountry are here because they are DERIVED from their
// …CodeA3 counterparts, not because the API rejects them.
//
// Note the absence of "version": unlike customer profiles, brands have no
// optimistic-locking token at all.
var brandReadOnlyFields = []string{
	"accounts", "bandwidthId", "brandId", "brandIdentityStatus", "brandRelationship",
	"authenticationStatus", "businessContactEmailVerifiedDate", "createdDate",
	"modifiedDate", "evpVettingScore", "imported", "universalEin", "country",
	"einIssuingCountry", "russell3000", "governmentEntity", "section527",
	"taxExemptStatus", "politicalCommitteeLocale", "referenceId",
}

// BrandUpdateFieldFlags are every flag `brand update` accepts, in CLI naming.
// BuildBrandUpdateRequest keys its changed map on exactly these names.
// customerProfileId is absent: the update schema does not accept it, and a
// profile backs exactly one brand for the brand's whole life.
var BrandUpdateFieldFlags = []string{
	"brand-type", "display-name", "company-name", "street", "city", "state",
	"postal-code", "country-code-a3", "phone", "email", "vertical", "ein",
	"ein-issuing-country-code-a3", "website", "stock-symbol", "stock-exchange",
	"alt-business-id", "alt-business-id-type", "business-contact-email",
	"first-name", "last-name", "mobile-phone", "ip-address",
}

// brandFlagToField maps a CLI flag name to the JSON key it writes.
var brandFlagToField = map[string]string{
	"brand-type": "brandType", "display-name": "displayName",
	"company-name": "companyName", "street": "street", "city": "city",
	"state": "state", "postal-code": "postalCode", "country-code-a3": "countryCodeA3",
	"phone": "phone", "email": "email", "vertical": "vertical", "ein": "ein",
	"ein-issuing-country-code-a3": "einIssuingCountryCodeA3", "website": "website",
	"stock-symbol": "stockSymbol", "stock-exchange": "stockExchange",
	"alt-business-id": "altBusinessId", "alt-business-id-type": "altBusinessIdType",
	"business-contact-email": "businessContactEmail", "first-name": "firstName",
	"last-name": "lastName", "mobile-phone": "mobilePhone", "ip-address": "ipAddress",
}

// BrandUpdateOptions is the flag surface of `band tendlc brand update`.
// Whether a field was explicitly set is tracked separately in the changed map,
// because an empty string is a legitimate value meaning "clear this".
type BrandUpdateOptions struct {
	BrandType     string
	DisplayName   string
	CompanyName   string
	Street        string
	City          string
	State         string
	PostalCode    string
	CountryCodeA3 string
	Phone         string
	Email         string
	Vertical      string
	EIN           string

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

// value returns the option value for a CLI flag name.
func (o BrandUpdateOptions) value(flag string) string {
	switch flag {
	case "brand-type":
		return o.BrandType
	case "display-name":
		return o.DisplayName
	case "company-name":
		return o.CompanyName
	case "street":
		return o.Street
	case "city":
		return o.City
	case "state":
		return o.State
	case "postal-code":
		return o.PostalCode
	case "country-code-a3":
		return o.CountryCodeA3
	case "phone":
		return o.Phone
	case "email":
		return o.Email
	case "vertical":
		return o.Vertical
	case "ein":
		return o.EIN
	case "ein-issuing-country-code-a3":
		return o.EINIssuingCountryCodeA3
	case "website":
		return o.Website
	case "stock-symbol":
		return o.StockSymbol
	case "stock-exchange":
		return o.StockExchange
	case "alt-business-id":
		return o.AltBusinessID
	case "alt-business-id-type":
		return o.AltBusinessIDType
	case "business-contact-email":
		return o.BusinessContactEmail
	case "first-name":
		return o.FirstName
	case "last-name":
		return o.LastName
	case "mobile-phone":
		return o.MobilePhone
	case "ip-address":
		return o.IPAddress
	}
	return ""
}

// BuildBrandUpdateRequest produces a full-replacement PUT body that cannot
// drop fields the CLI does not model.
//
// PUT replaces the whole resource, so anything missing from the body is nulled
// server-side. Building the body from a typed struct would therefore delete
// every production field we never modeled — and the brand response already
// carries several the published schema omits. So the body starts as a copy of
// what the API just gave us, read-only fields are removed, and only
// explicitly-changed flags are overlaid. Validation stays typed; the payload
// stays lossless.
func BuildBrandUpdateRequest(current map[string]any, o BrandUpdateOptions, changed map[string]bool) (map[string]any, error) {
	if current == nil {
		return nil, fmt.Errorf("no current resource to update from")
	}

	body := deepCopyBrandMap(current)
	for _, ro := range brandReadOnlyFields {
		delete(body, ro)
	}

	for _, flag := range BrandUpdateFieldFlags {
		if !changed[flag] {
			continue
		}
		field := brandFlagToField[flag]
		if v := o.value(flag); v != "" {
			body[field] = v
		} else {
			// An explicitly empty value clears the field. It must go over the
			// wire as JSON null: the API rejects "" on at least one field and
			// accepts null.
			body[field] = nil
		}
	}

	if err := ValidateBrandUpdate(body); err != nil {
		return nil, err
	}
	return body, nil
}

// ValidateBrandUpdate checks the fully overlaid PUT body — the object about to
// go over the wire — not the options struct. Catching a cleared required field
// here makes the failure a local, zero-request FlagError (exit 6) rather than
// a raw 400 from the API.
//
// The universal fields (required on every brand type) are checked first. On
// top of those, the same per-type tier ValidateBrandCreate enforces
// (registeredEntityRequiredFlags for every type except SOLE_PROPRIETOR;
// registeredEntityRequiredFlags + publicProfitRequiredFlags for
// PUBLIC_PROFIT, both from brandoptions.go, reused rather than duplicated)
// applies here too — clearing --vertical on a PRIVATE_PROFIT brand or
// --website on a PUBLIC_PROFIT one is exactly the mistake ValidateBrandCreate
// already catches on create, and update must catch it the same way rather
// than letting it reach the API as a raw 400. brandType is read from the
// completed BODY, not from the options struct: the consequence attaches to
// what the brand IS right now, the same reasoning IdentityFieldsChanged uses.
// SOLE_PROPRIETOR still skips the per-type tier — its rules are
// account-gated and unobservable on any account we have, and inventing them
// would reject requests production accepts. The tier is also skipped
// whenever brandType is itself missing or invalid: which tier would apply
// isn't known.
//
// brandType also gets the same enum check ValidateBrandCreate runs via
// validBrandType: without it, a --brand-type typo exits 6 on create but
// reaches the API and comes back as a raw 400 on update. As in
// ValidateBrandCreate, an invalid brand type does not short-circuit — it is
// aggregated with any cleared required fields (universal AND per-type) into
// one error, the way the API reports every violation in one 400.
func ValidateBrandUpdate(body map[string]any) error {
	required := [][2]string{
		{"brand-type", "brandType"},
		{"display-name", "displayName"},
		{"street", "street"},
		{"city", "city"},
		{"state", "state"},
		{"postal-code", "postalCode"},
		{"country-code-a3", "countryCodeA3"},
		{"phone", "phone"},
		{"email", "email"},
	}
	var cleared []string
	var invalidBrandType bool
	brandTypeMissing := false
	brandType := ""
	for _, p := range required {
		s, ok := body[p[1]].(string)
		if !ok || s == "" {
			cleared = append(cleared, p[0])
			if p[1] == "brandType" {
				brandTypeMissing = true
			}
			continue
		}
		if p[1] == "brandType" {
			brandType = s
			if !validBrandType(s) {
				invalidBrandType = true
			}
		}
	}

	if !brandTypeMissing && !invalidBrandType {
		switch brandType {
		case "PRIVATE_PROFIT", "NON_PROFIT", "GOVERNMENT":
			cleared = append(cleared, clearedTierFields(body, registeredEntityRequiredFlags)...)
		case "PUBLIC_PROFIT":
			cleared = append(cleared, clearedTierFields(body, registeredEntityRequiredFlags)...)
			cleared = append(cleared, clearedTierFields(body, publicProfitRequiredFlags)...)
			// SOLE_PROPRIETOR (and any other valid type): no per-type tier to check.
		}
	}

	if invalidBrandType {
		enumMsg := "--brand-type must be one of: " + strings.Join(BrandTypes, ", ")
		if len(cleared) > 0 {
			return cmdutil.NewFlagError(enumMsg + "; these fields are required on every brand and cannot be cleared: --" +
				strings.Join(cleared, ", --"))
		}
		return cmdutil.NewFlagError(enumMsg)
	}

	if len(cleared) > 0 {
		return cmdutil.NewFlagError("these fields are required on every brand and cannot be cleared: --" +
			strings.Join(cleared, ", --"))
	}
	return nil
}

// clearedTierFields returns the flags in tier (a per-type requirement list
// from brandoptions.go) whose backing field is missing or empty in body —
// i.e. would clear a field production requires for the brand's current type
// on this full-replacement PUT.
func clearedTierFields(body map[string]any, tier []string) []string {
	var out []string
	for _, flag := range tier {
		s, ok := body[brandFlagToField[flag]].(string)
		if !ok || s == "" {
			out = append(out, flag)
		}
	}
	return out
}

// identityFlags are the flags whose change can trigger a $4 fee, reset the
// brand's identity status, or revoke Auth+ compliance.
//
// Fee detection is not attempted: whether the API actually charges depends on
// active campaigns and active vettings, neither present in the brand detail
// response. Determining it would need two more reads, a definition of
// "active", and would still race. So the contract is over-asking — --confirm
// is required whenever one of these is explicitly changed, charged or not.
// Over-asking is safe; under-asking bills the customer.
var identityFlags = []string{
	"company-name", "brand-type", "ein", "ein-issuing-country-code-a3", "mobile-phone",
}

// IdentityFieldsChanged returns the identity-affecting flags the caller
// explicitly set, sorted, or nil when none were.
//
// business-contact-email is conditional: changing it revokes Auth+ compliance
// on PUBLIC_PROFIT brands only, so it counts only when the CURRENT brand is
// PUBLIC_PROFIT. Reading brandType from current, not from the options, is
// deliberate — the consequence attaches to what the brand is right now.
func IdentityFieldsChanged(current map[string]any, changed map[string]bool) []string {
	var out []string
	for _, f := range identityFlags {
		if changed[f] {
			out = append(out, f)
		}
	}
	if changed["business-contact-email"] {
		if bt, _ := current["brandType"].(string); bt == "PUBLIC_PROFIT" {
			out = append(out, "business-contact-email")
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}

// brandNeverDropFields is every JSON body key any `brand update` flag can
// write — i.e. every value in brandFlagToField. UpdateBrand passes the
// result to putReplaceWithReadOnlyRetry as the set of fields the retry must
// never drop: a field the CLI models at all is mutable customer data (see
// putReplaceWithReadOnlyRetry's INVARIANT and the --website example there),
// so it is never eligible to be silently stripped and re-sent — regardless
// of whether the caller's most recent invocation happened to touch it.
func brandNeverDropFields() map[string]bool {
	out := make(map[string]bool, len(brandFlagToField))
	for _, field := range brandFlagToField {
		out[field] = true
	}
	return out
}

// deepCopyBrandMap copies m so the result shares no mutable structure with it.
// current is read from an api.Envelope the caller may reuse, so a shallow copy
// would leave nested values (brand.accounts is a []any of maps) aliased
// between the outgoing body and the caller's data.
func deepCopyBrandMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopyBrandValue(v)
	}
	return out
}

func deepCopyBrandValue(v any) any {
	switch vv := v.(type) {
	case map[string]any:
		return deepCopyBrandMap(vv)
	case []any:
		out := make([]any, len(vv))
		for i, e := range vv {
			out[i] = deepCopyBrandValue(e)
		}
		return out
	default:
		return v
	}
}
