package tendlc

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	cpsvc "github.com/Bandwidth/cli/internal/customerprofile"
	"github.com/Bandwidth/cli/internal/output"
	tendlcsvc "github.com/Bandwidth/cli/internal/tendlc"
)

var (
	brandCreateOpts    tendlcsvc.BrandCreateOptions
	brandCreateWait    bool
	brandCreateTimeout int
)

// brandCreatePollInterval is how often --wait re-checks the brand while it
// registers. Brand identity verification runs through TCR, an external
// registry, so — unlike the 2-second intervals used for Bandwidth-internal
// polls elsewhere in this CLI — a slower interval is appropriate here.
const brandCreatePollInterval = 5 * time.Second

// customerProfileService builds a customer-profile Service for the create
// pre-flight check. A package-level seam of its own, separate from this
// package's `service` seam (declared in status.go), so tests can stub the
// profile read independently of the brand-create request itself.
var customerProfileService = func(cmd *cobra.Command) (*cpsvc.Service, error) {
	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return nil, err
	}
	return cpsvc.NewService(client, acctID), nil
}

func init() {
	f := brandCreateCmd.Flags()
	f.StringVar(&brandCreateOpts.CustomerProfileID, "customer-profile-id", "", "Customer profile backing this brand (required)")
	f.StringVar(&brandCreateOpts.BrandType, "brand-type", "", "Brand entity type: PRIVATE_PROFIT, PUBLIC_PROFIT, NON_PROFIT, GOVERNMENT, SOLE_PROPRIETOR (required)")
	f.StringVar(&brandCreateOpts.DisplayName, "display-name", "", "Display name (required)")
	f.StringVar(&brandCreateOpts.CompanyName, "company-name", "", "Legal company name")
	f.StringVar(&brandCreateOpts.Street, "street", "", "Street address (required)")
	f.StringVar(&brandCreateOpts.City, "city", "", "City (required)")
	f.StringVar(&brandCreateOpts.State, "state", "", "State or province (required)")
	f.StringVar(&brandCreateOpts.PostalCode, "postal-code", "", "Postal code (required)")
	f.StringVar(&brandCreateOpts.CountryCodeA3, "country-code-a3", "", "ISO 3166-1 alpha-3 country code (required)")
	f.StringVar(&brandCreateOpts.Phone, "phone", "", "Business phone number (required)")
	f.StringVar(&brandCreateOpts.Email, "email", "", "Business email address (required)")
	f.StringVar(&brandCreateOpts.Vertical, "vertical", "", "Industry vertical")
	f.StringVar(&brandCreateOpts.EIN, "ein", "", "Employer Identification Number")
	f.StringVar(&brandCreateOpts.EINIssuingCountryCodeA3, "ein-issuing-country-code-a3", "", "Country that issued the EIN")
	f.StringVar(&brandCreateOpts.Website, "website", "", "Business website URL")
	f.StringVar(&brandCreateOpts.StockSymbol, "stock-symbol", "", "Stock ticker symbol")
	f.StringVar(&brandCreateOpts.StockExchange, "stock-exchange", "", "Stock exchange")
	f.StringVar(&brandCreateOpts.BusinessContactEmail, "business-contact-email", "", "Business contact email")
	f.StringVar(&brandCreateOpts.AltBusinessID, "alt-business-id", "", "Alternate business identifier (e.g. DUNS number)")
	f.StringVar(&brandCreateOpts.AltBusinessIDType, "alt-business-id-type", "", "Type of the alternate business identifier")
	f.StringVar(&brandCreateOpts.FirstName, "first-name", "", "Contact first name (sole proprietor)")
	f.StringVar(&brandCreateOpts.LastName, "last-name", "", "Contact last name (sole proprietor)")
	f.StringVar(&brandCreateOpts.MobilePhone, "mobile-phone", "", "Contact mobile phone (sole proprietor)")
	f.StringVar(&brandCreateOpts.IPAddress, "ip-address", "", "IP address the form was completed from (sole proprietor)")
	f.BoolVar(&brandCreateWait, "wait", false, "Block until the brand's identity status reaches a terminal state")
	f.IntVar(&brandCreateTimeout, "timeout", 300, "Seconds to wait when --wait is set")
	brandCmd.AddCommand(brandCreateCmd)
	brandCmd.AddCommand(brandRefreshCmd)
}

var brandCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Register a 10DLC brand",
	Long: `Registers a 10DLC brand backed by an existing customer profile.

A customer profile backs exactly one brand — create a fresh profile per
brand with 'band customer-profile create'; reusing one fails at brand
creation.

Before submitting, this command reads the customer profile named by
--customer-profile-id. Measured against production, the create endpoint
itself does not reject a garbage or typo'd profile ID — it silently
discards it and creates an orphan brand with no profile association that
can never verify. So a 404 on that read stops the create here, naming the
bad ID. A 403 does not stop it: Customer Profiles Access is a role separate
from Campaign Management, so a caller entitled to create brands must not be
blocked by a check they lack permission to run — it proceeds with a
one-line warning on stderr instead. Any other pre-flight failure degrades
the same way; this check is a guard, not a gate.

This is a non-idempotent, billable write. After an ambiguous failure — a
timeout or a dropped connection — do not blindly retry: list brands
filtered by --customer-profile-id-contains and reconcile against what you
submitted first. Retrying blind risks a second brand against the same profile.`,
	Example: `  band tendlc brand create --customer-profile-id CP123 --brand-type PRIVATE_PROFIT \
    --display-name "Acme Corp" --company-name "Acme Corporation" \
    --street "123 Main St" --city Raleigh --state NC --postal-code 27601 \
    --country-code-a3 USA --phone +18885551234 --email ops@acme.com \
    --vertical RETAIL --ein 123456789 --ein-issuing-country-code-a3 USA

  band tendlc brand create --customer-profile-id CP123 --brand-type PRIVATE_PROFIT \
    ... --wait --timeout 600`,
	// No positional args: this is a non-idempotent, billable create, so a
	// stray positional (e.g. a typo'd second word meant for another flag)
	// must be rejected rather than silently ignored and creating an
	// unintended brand — see TestBrandCommandsRejectStrayPositionals's
	// comment on the PR 2 incident this guards against.
	Args: cobra.NoArgs,
	// Required-ness is enforced in RunE, not via MarkFlagRequired: cobra
	// rejects before RunE, which reports one flag at a time and would block a
	// future interactive prompt from filling them in.
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := tendlcsvc.ValidateBrandCreate(brandCreateOpts); err != nil {
			return err
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		if err := preflightCustomerProfile(cmd, brandCreateOpts.CustomerProfileID); err != nil {
			return err
		}

		env, err := svc.CreateBrand(tendlcsvc.BuildBrandCreateRequest(brandCreateOpts))
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}

		receipt, bandwidthID, err := buildAcceptedReceipt(cmd, env)
		if err != nil {
			return err
		}

		if !brandCreateWait {
			format, _ := cmdutil.OutputFlags(cmd)
			return output.Stdout(format, receipt)
		}

		target := pollTarget{
			Noun:  "brand",
			Fetch: fetchBrand(svc, bandwidthID),
			Classify: func(o map[string]any) tendlcsvc.StateClass {
				status, _ := o["brandIdentityStatus"].(string)
				return tendlcsvc.ClassifyBrandIdentity(status)
			},
			Remediate: func(o map[string]any) string {
				status, _ := o["brandIdentityStatus"].(string)
				return tendlcsvc.BrandRemediation(status)
			},
			LastSeenStatus: func(o map[string]any) string {
				status, _ := o["brandIdentityStatus"].(string)
				return status
			},
		}
		// UNVERIFIED now polls to timeout instead of failing fast (see
		// ClassifyBrandIdentity), which makes a timeout the normal way a
		// truly-failed registration surfaces. The receipt has to carry that
		// weight, so a note is added directly to it here — advisory only,
		// and it never reaches the success or business-failure (ERROR) output
		// paths, only the timeout/transport-error receipt (see awaitTerminal's
		// emitReceipt).
		receipt["note"] = "if this timed out at UNVERIFIED, the brand may still be registering with TCR " +
			"rather than having failed. Check 'band tendlc brand get " + bandwidthID +
			"' for its current status, or 'band tendlc brand history " + bandwidthID + "' for the full history."
		return awaitTerminal(cmd, target, receipt, time.Duration(brandCreateTimeout)*time.Second, brandCreatePollInterval)
	},
}

var brandRefreshCmd = &cobra.Command{
	Use:   "refresh <brand-id>",
	Short: "Re-pull a brand's current state from TCR",
	Long: `Refreshes a brand by re-pulling its current state from TCR — for import
and direct customers alike.

This is not a create: it posts to the same POST /brands endpoint create
uses, but a body containing only brandId is what makes it a refresh instead
of a new registration. Sending any other key would turn it back into a
create, so nothing else is added. Use it after making a change directly in
TCR, or to pick up a brandId that was still null when the brand was
created.`,
	Example: `  band tendlc brand refresh BGJR2BA --plain`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		env, err := svc.CreateBrand(tendlcsvc.BuildBrandRefreshRequest(args[0]))
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}
		receipt, _, err := buildAcceptedReceipt(cmd, env)
		if err != nil {
			return err
		}
		format, _ := cmdutil.OutputFlags(cmd)
		return output.Stdout(format, receipt)
	},
}

// preflightCustomerProfile reads the customer profile named by profileID
// before create writes the brand — see brandCreateCmd's Long for why. It
// degrades rather than blocks: only a definitive 404 (the profile does not
// exist) stops the create. A 403, a failure to build the profile service, or
// any other error is reported on stderr and swallowed, because this check is
// a guard against a specific typo, not a gate on the whole command.
func preflightCustomerProfile(cmd *cobra.Command, profileID string) error {
	cpSvc, err := customerProfileService(cmd)
	if err == nil {
		_, err = cpSvc.Get(profileID)
	}
	if err == nil {
		return nil
	}
	if isNotFound(err) {
		return fmt.Errorf("customer profile %q not found — run 'band customer-profile list' to see valid profile IDs (%w)", profileID, err)
	}
	cmd.PrintErrf("warning: could not verify customer profile %q before creating the brand (%v); proceeding anyway\n", profileID, err)
	return nil
}

// buildAcceptedReceipt turns a brand-write acceptance response — the POST
// /brands 202 that both create and refresh get, or the PUT /brands/{id}
// response update gets — into the receipt shape all three print:
// {bandwidthId, brandId (if present), status, resume}. bandwidthId exists
// immediately; brandId is assigned by TCR and is commonly absent until
// registration completes, so it is omitted entirely rather than sent as null.
//
// If the response carries no bandwidthId, there is no ID to poll or resume
// with, so the caller must not proceed. This prints whatever the body
// actually was via output.Stdout, not StdoutAuto: env.Data is real API data,
// not a synthetic receipt, but it can still be a single-key map (the orphan
// brand's {"accounts":[...]} body is exactly one), and FlattenResponse
// unwraps any single-key map — under --plain that would drop the key and
// print a bare array instead of the object it came from. See async.go's
// emitReceipt for the same reasoning applied to synthetic receipts.
func buildAcceptedReceipt(cmd *cobra.Command, env *api.Envelope) (receipt map[string]any, bandwidthID string, err error) {
	obj, objErr := env.Object()
	bandwidthID, _ = obj["bandwidthId"].(string)
	if objErr != nil || bandwidthID == "" {
		format, _ := cmdutil.OutputFlags(cmd)
		if writeErr := output.Stdout(format, env.Data); writeErr != nil {
			cmd.PrintErrln(fmt.Sprintf("writing response: %v", writeErr))
		}
		return nil, "", fmt.Errorf("brand response did not include a bandwidthId")
	}

	receipt = map[string]any{
		"bandwidthId": bandwidthID,
		"status":      "accepted",
		"resume":      "band tendlc brand get " + bandwidthID,
	}
	if brandID, ok := obj["brandId"].(string); ok && brandID != "" {
		receipt["brandId"] = brandID
	}
	return receipt, bandwidthID, nil
}
