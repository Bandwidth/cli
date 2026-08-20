package tendlc

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	brandListLimit                     int
	brandListOffset                    int
	brandListAll                       bool
	brandListCustomerProfileIDContains string
	brandListBrandIDContains           string
	brandListIdentityStatus            string
	brandListBrandType                 string
	brandListCompanyNameContains       string
	brandListDisplayNameContains       string
)

func init() {
	f := brandListCmd.Flags()
	f.IntVar(&brandListLimit, "limit", 50, "Page size")
	f.IntVar(&brandListOffset, "offset", 0, "Pagination offset")
	f.BoolVar(&brandListAll, "all", false, "Fetch every page (cannot be combined with --offset)")
	f.StringVar(&brandListCustomerProfileIDContains, "customer-profile-id-contains", "",
		"Filter to brands whose customer profile ID contains this substring (e.g. 9900000 also matches 9900000-1); the API has no exact-match operator for this field, see 'brand list --help' notes")
	f.StringVar(&brandListBrandIDContains, "brand-id-contains", "",
		"Filter by TCR brand ID substring (e.g. BEXMPL1 also matches BEXMPL12); the API has no exact-match operator for this field, use 'brand get <id>' for a single brand")
	f.StringVar(&brandListIdentityStatus, "identity-status", "",
		"Filter by brandIdentityStatus, exact match (VERIFIED, VETTED_VERIFIED, UNVERIFIED, ERROR; REGISTERING is documented but never observed on the read path, so filtering on it returns nothing)")
	f.StringVar(&brandListBrandType, "brand-type", "", "Filter by brand type, exact match")
	f.StringVar(&brandListCompanyNameContains, "company-name-contains", "", "Filter by legal company name substring")
	f.StringVar(&brandListDisplayNameContains, "display-name-contains", "", "Filter by display name substring")
	brandCmd.AddCommand(brandListCmd)
}

var brandListCmd = &cobra.Command{
	Use:   "list",
	Short: "List 10DLC brands",
	Long: `Lists brands on the account.

This is a summary projection — 12 keys per brand, versus the 46 keys 'brand
get' returns. A field that is missing here may simply not be part of the
listing projection, not null on the brand; use 'brand get <id>' for the full
resource.

There is no --bandwidth-id filter: measured against production,
bandwidthId[eq] is accepted and silently ignored, returning every brand
rather than filtering. Use 'brand get <bandwidth-id>' to fetch one directly.`,
	Example: `  band tendlc brand list --plain
  band tendlc brand list --all --plain
  band tendlc brand list --identity-status VERIFIED --plain`,
	// No positional args: list takes only flags.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Detected via Changed so that an explicit --offset 0 also conflicts.
		if brandListAll && cmd.Flags().Changed("offset") {
			return cmdutil.NewFlagError("--all fetches every page, so it cannot be combined with --offset")
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}

		// Measured against production (test account, 10 brands): GET /brands
		// accepts the `eq` operator on every field below and silently drops
		// it -- brandId[eq]=X returns all 10 brands, not the one with that
		// ID. `contains` is the only operator that actually filters there:
		//
		//   brandId[contains]             -> 1 of 10  (substring match)
		//   customerProfileId[contains]   -> 1 of 10  (substring match)
		//   brandIdentityStatus[contains] -> 3 of 10  ("VETTED" matched the 3 VETTED_VERIFIED brands)
		//   brandType[contains]           -> 5 of 10
		//   companyName[contains]         -> 9 of 10  (already implemented this way)
		//   displayName[contains]         -> 4 of 10  (already implemented this way)
		//
		// Do not "correct" these back to OpEq: eq looks more right and is
		// silently wrong -- it returns a 200 with every brand on the account
		// and no error to signal the filter did nothing. This is the same
		// failure already known for bandwidthId (see the "no --bandwidth-id
		// filter" note above); it was wrongly assumed to be specific to that
		// field.
		//
		// contains is a substring match, not equality, so
		// --brand-id-contains and --customer-profile-id-contains are named
		// for what they actually do (BEXMPL1 also matches BEXMPL12) rather
		// than implying an exact-match --brand-id/--customer-profile-id that
		// the API cannot perform.
		//
		// --identity-status and --brand-type are closed enums, so a
		// contains match has a sharper trap than an arbitrary substring:
		// brandIdentityStatus[contains]=VERIFIED matches VERIFIED,
		// VETTED_VERIFIED, *and* UNVERIFIED -- the most obvious query a
		// caller would type ("give me the VERIFIED ones") would silently
		// include the exact opposite status. Because the two fields are
		// closed enums, exactness is well-defined and cheap, so the results
		// are narrowed server-side with contains and then filtered
		// client-side (filterExactField, below) for an exact match, so the
		// flag keeps its exact-match meaning even though the wire request
		// underneath is contains.
		var filters []api.Filter
		if brandListCustomerProfileIDContains != "" {
			filters = append(filters, api.Filter{Field: "customerProfileId", Op: api.OpContains, Value: brandListCustomerProfileIDContains})
		}
		if brandListBrandIDContains != "" {
			filters = append(filters, api.Filter{Field: "brandId", Op: api.OpContains, Value: brandListBrandIDContains})
		}
		if brandListIdentityStatus != "" {
			filters = append(filters, api.Filter{Field: "brandIdentityStatus", Op: api.OpContains, Value: brandListIdentityStatus})
		}
		if brandListBrandType != "" {
			filters = append(filters, api.Filter{Field: "brandType", Op: api.OpContains, Value: brandListBrandType})
		}
		if brandListCompanyNameContains != "" {
			filters = append(filters, api.Filter{Field: "companyName", Op: api.OpContains, Value: brandListCompanyNameContains})
		}
		if brandListDisplayNameContains != "" {
			filters = append(filters, api.Filter{Field: "displayName", Op: api.OpContains, Value: brandListDisplayNameContains})
		}

		format, plain := cmdutil.OutputFlags(cmd)

		if !brandListAll {
			env, err := svc.ListBrands(brandListLimit, brandListOffset, filters)
			if err != nil {
				return roleGateError(err, "Campaign Management")
			}
			items, err := env.List()
			if err != nil {
				return err
			}
			items = filterExactField(items, "brandIdentityStatus", brandListIdentityStatus)
			items = filterExactField(items, "brandType", brandListBrandType)
			// warnIfTruncated runs after the exact-match narrowing so "showing
			// X" reflects what is actually printed. Its "of Y" total, however,
			// still comes straight from the server and reflects the broader
			// contains match, not the exact one -- if --identity-status or
			// --brand-type is set, Y can overstate how many brands remain on
			// later pages that exactly match. --all does not have this gap:
			// it walks every page before narrowing, so its count is exact.
			warnIfTruncated(cmd, env, brandListOffset, len(items), "brands")
			return output.StdoutPlainList(format, plain, items)
		}

		var all []any
		err = api.ForEachPage(func(limit, offset int) (*api.Envelope, error) {
			return svc.ListBrands(limit, offset, filters)
		}, brandListLimit, func(batch []any) error {
			all = append(all, batch...)
			return nil
		})
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}
		if all == nil {
			all = []any{}
		}
		all = filterExactField(all, "brandIdentityStatus", brandListIdentityStatus)
		all = filterExactField(all, "brandType", brandListBrandType)
		return output.StdoutPlainList(format, plain, all)
	},
}

// filterExactField keeps only the items whose field is exactly value,
// dropping the rest. value == "" is a no-op (the caller didn't set that
// flag). It exists because the API only offers `contains` on
// brandIdentityStatus/brandType (see the RunE comment above), and contains
// is not precise enough for those two closed enums: VERIFIED[contains] also
// matches VETTED_VERIFIED and UNVERIFIED. Items whose field is missing or
// not a string are dropped rather than kept, since a promised exact match
// should never silently include something that couldn't be compared.
func filterExactField(items []any, field, value string) []any {
	if value == "" {
		return items
	}
	out := items[:0:0]
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if s, ok := m[field].(string); ok && s == value {
			out = append(out, item)
		}
	}
	return out
}
