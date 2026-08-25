package tendlc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	tendlcsvc "github.com/Bandwidth/cli/internal/tendlc"
)

var (
	brandUpdateOpts    tendlcsvc.BrandUpdateOptions
	brandUpdateConfirm bool
)

func init() {
	f := brandUpdateCmd.Flags()
	f.StringVar(&brandUpdateOpts.BrandType, "brand-type", "", "Brand entity type: PRIVATE_PROFIT, PUBLIC_PROFIT, NON_PROFIT, GOVERNMENT, SOLE_PROPRIETOR")
	f.StringVar(&brandUpdateOpts.DisplayName, "display-name", "", "Display name")
	f.StringVar(&brandUpdateOpts.CompanyName, "company-name", "", "Legal company name")
	f.StringVar(&brandUpdateOpts.Street, "street", "", "Street address")
	f.StringVar(&brandUpdateOpts.City, "city", "", "City")
	f.StringVar(&brandUpdateOpts.State, "state", "", "State or province")
	f.StringVar(&brandUpdateOpts.PostalCode, "postal-code", "", "Postal code")
	f.StringVar(&brandUpdateOpts.CountryCodeA3, "country-code-a3", "", "ISO 3166-1 alpha-3 country code")
	f.StringVar(&brandUpdateOpts.Phone, "phone", "", "Business phone number")
	f.StringVar(&brandUpdateOpts.Email, "email", "", "Business email address")
	f.StringVar(&brandUpdateOpts.Vertical, "vertical", "", "Industry vertical")
	f.StringVar(&brandUpdateOpts.EIN, "ein", "", "Employer Identification Number")
	f.StringVar(&brandUpdateOpts.EINIssuingCountryCodeA3, "ein-issuing-country-code-a3", "", "Country that issued the EIN")
	f.StringVar(&brandUpdateOpts.Website, "website", "", "Business website URL")
	f.StringVar(&brandUpdateOpts.StockSymbol, "stock-symbol", "", "Stock ticker symbol")
	f.StringVar(&brandUpdateOpts.StockExchange, "stock-exchange", "", "Stock exchange")
	f.StringVar(&brandUpdateOpts.BusinessContactEmail, "business-contact-email", "", "Business contact email")
	f.StringVar(&brandUpdateOpts.AltBusinessID, "alt-business-id", "", "Alternate business identifier (e.g. DUNS number)")
	f.StringVar(&brandUpdateOpts.AltBusinessIDType, "alt-business-id-type", "", "Type of the alternate business identifier")
	f.StringVar(&brandUpdateOpts.FirstName, "first-name", "", "Contact first name (sole proprietor)")
	f.StringVar(&brandUpdateOpts.LastName, "last-name", "", "Contact last name (sole proprietor)")
	f.StringVar(&brandUpdateOpts.MobilePhone, "mobile-phone", "", "Contact mobile phone (sole proprietor)")
	f.StringVar(&brandUpdateOpts.IPAddress, "ip-address", "", "IP address the form was completed from (sole proprietor)")
	f.BoolVar(&brandUpdateConfirm, "confirm", false, "Required when the change affects the brand's identity verification, mobile-phone, or business-contact-email (PUBLIC_PROFIT).")
	brandCmd.AddCommand(brandUpdateCmd)
}

var brandUpdateCmd = &cobra.Command{
	Use:   "update <brand-id>",
	Short: "Update a 10DLC brand",
	Long: `Updates a 10DLC brand.

The API replaces the whole record on update, so this command reads the brand
first and sends it back with your changes applied. Fields you do not pass are
preserved; passing a flag with an empty value clears that field (a field
required on every brand cannot be cleared this way).

This command prints an acceptance receipt, not the updated brand: measured
against production, the PUT here returns a bare {bandwidthId, brandId}
acceptance, and the change itself takes roughly 5 minutes to be reflected in
'brand get' or 'brand history' — modifiedDate and the activity log both lag
behind. Re-check after a few minutes to confirm the change actually applied.

No --wait: unlike 'brand create', this write lands against a brand that is
usually already in a terminal identity state, so polling for one here would
return immediately and report success before the change actually took
effect. The ~5 minute apply-latency above confirms this rather than merely
motivating it: a --wait here would poll a brand still holding its
pre-update state and report success before anything actually changed.

Some changes need --confirm: changing company-name, brand-type, ein, or
ein-issuing-country-code-a3 resubmits the brand for identity verification
(may incur a $4 fee, resets brandIdentityStatus toward re-registration —
documented as REGISTERING, but it reads back as UNVERIFIED until TCR
responds — and is rejected outright if the brand has an active campaign or
an active Standard/Enhanced/Political vetting). Changing mobile-phone sets
identity status to UNVERIFIED. Changing business-contact-email on a
PUBLIC_PROFIT brand revokes Auth+ compliance.`,
	Example: `  band tendlc brand update BEXMPL6 --website "https://acme.example" --plain
  band tendlc brand update BEXMPL6 --company-name "Acme Corp 2" --confirm --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		changed := map[string]bool{}
		any := false
		for _, name := range tendlcsvc.BrandUpdateFieldFlags {
			if cmd.Flags().Changed(name) {
				changed[name] = true
				any = true
			}
		}
		if !any {
			return cmdutil.NewFlagError(
				"nothing to update — pass at least one of " + flagList(tendlcsvc.BrandUpdateFieldFlags))
		}

		svc, err := service(cmd)
		if err != nil {
			return err
		}

		env, err := svc.GetBrand(cmd.Context(), args[0])
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}
		current, err := env.Object()
		if err != nil {
			return err
		}

		// The --confirm gate for identity-affecting fields fires HERE, after the
		// GET above, rather than before it — this is the one confirm gate in the
		// whole PR that is not entirely zero-request. That is deliberate, not an
		// oversight to "fix" into a pre-GET check: IdentityFieldsChanged's
		// business-contact-email/PUBLIC_PROFIT condition needs the brand's
		// CURRENT type, which is only known once this GET has returned. Moving
		// this check earlier to make refusal zero-request-of-any-kind would
		// silently stop detecting that case — a caller changing
		// business-contact-email on a PUBLIC_PROFIT brand would sail through
		// with no Auth+ warning at all. The property that actually matters here
		// — zero *write* requests before --confirm is satisfied — still holds:
		// this gate can only ever have caused a GET, never the PUT below.
		if fields := tendlcsvc.IdentityFieldsChanged(current, changed); len(fields) > 0 {
			if err := requireConfirm(brandUpdateConfirm, identityConfirmMessage(args[0], fields)); err != nil {
				return err
			}
		}

		body, err := tendlcsvc.BuildBrandUpdateRequest(current, brandUpdateOpts, changed)
		if err != nil {
			return err
		}

		updated, err := svc.UpdateBrand(cmd.Context(), args[0], body)
		if err != nil {
			return brandUpdateConflictHint(args[0], err)
		}

		// PUT /brands/{id} returns an ACCEPTANCE, not the updated resource —
		// measured against production: the body is a bare {bandwidthId,
		// brandId}, and the change itself takes roughly 5 minutes to be
		// reflected in a follow-up 'brand get' or 'brand history'. Printing
		// updated.Object() here would hand back that 2-key acceptance as
		// though it were the brand, with nothing telling the caller a change
		// is still pending — so this reuses buildAcceptedReceipt (the same
		// shape 'brand create'/'brand refresh' print) and adds a note naming
		// the latency, rather than printing the raw response as the result.
		receipt, bandwidthID, err := buildAcceptedReceipt(cmd, updated)
		if err != nil {
			return err
		}
		receipt["note"] = "this is an acceptance, not the updated brand: production takes about 5 " +
			"minutes to apply the change (modifiedDate and the history log lag behind), so an " +
			"immediate 'brand get' may still show the pre-update value. Check 'band tendlc brand get " +
			bandwidthID + "' again shortly, or 'band tendlc brand history " + bandwidthID +
			"' for confirmation."
		format, _ := cmdutil.OutputFlags(cmd)
		return output.Stdout(format, receipt)
	},
}

// identityVerificationFields are the identity-affecting flags whose
// consequence is resubmission for identity verification — as opposed to
// mobile-phone and business-contact-email, which each have a distinct
// consequence named separately below.
var identityVerificationFields = map[string]bool{
	"company-name": true, "brand-type": true, "ein": true, "ein-issuing-country-code-a3": true,
}

// identityConfirmMessage builds the --confirm refusal for an identity-
// affecting change. fields is the (already sorted, already conditioned)
// output of IdentityFieldsChanged. Every consequence that applies is named —
// a generic "this changes something important" would tell the caller nothing
// about what they are agreeing to, and when several fields apply for
// different reasons, all of them must be named, not just the first match.
func identityConfirmMessage(brandID string, fields []string) string {
	var verification []string
	mobileChanged := false
	businessContactEmailChanged := false
	for _, f := range fields {
		switch {
		case identityVerificationFields[f]:
			verification = append(verification, f)
		case f == "mobile-phone":
			mobileChanged = true
		case f == "business-contact-email":
			businessContactEmailChanged = true
		}
	}

	var parts []string
	if len(verification) > 0 {
		parts = append(parts, fmt.Sprintf(
			"changing %s on brand %s resubmits it for identity verification: this may incur a $4 fee and resets brandIdentityStatus toward re-registration (it reads back as UNVERIFIED until TCR responds). If the brand has an active campaign or an active Standard/Enhanced/Political vetting, the API will reject the change outright.",
			strings.Join(verification, ", "), brandID))
	}
	if mobileChanged {
		parts = append(parts, "changing mobile-phone sets identity status to UNVERIFIED.")
	}
	if businessContactEmailChanged {
		parts = append(parts, "changing business-contact-email on a PUBLIC_PROFIT brand revokes Auth+ compliance; regaining it requires a new AUTHPLUS vetting and another 2FA email verification.")
	}
	parts = append(parts, "Pass --confirm to proceed.")
	return strings.Join(parts, " ")
}

// brandUpdateConflictHint maps a 409 on the update PUT to its specific cause:
// the brand exists in Bandwidth but TCR has not caught up with it yet. Any
// other error still goes through roleGateError, so a 403 on update gets the
// same targeted message every other tendlc write does.
func brandUpdateConflictHint(brandID string, err error) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 409 {
		return &cmdutil.ConflictError{
			Message: "brand " + brandID + " exists in Bandwidth but not yet in TCR; run 'band tendlc brand refresh " + brandID + "' and retry",
			Cause:   err,
		}
	}
	return roleGateError(err, "Campaign Management")
}
