package tendlc

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	brandListLimit               int
	brandListOffset              int
	brandListAll                 bool
	brandListCustomerProfileID   string
	brandListBrandID             string
	brandListIdentityStatus      string
	brandListBrandType           string
	brandListCompanyNameContains string
	brandListDisplayNameContains string
)

func init() {
	f := brandListCmd.Flags()
	f.IntVar(&brandListLimit, "limit", 50, "Page size")
	f.IntVar(&brandListOffset, "offset", 0, "Pagination offset")
	f.BoolVar(&brandListAll, "all", false, "Fetch every page (cannot be combined with --offset)")
	f.StringVar(&brandListCustomerProfileID, "customer-profile-id", "", "Filter to the brand backed by this customer profile")
	f.StringVar(&brandListBrandID, "brand-id", "", "Filter by TCR brand ID")
	f.StringVar(&brandListIdentityStatus, "identity-status", "", "Filter by brandIdentityStatus (REGISTERING, VERIFIED, VETTED_VERIFIED, UNVERIFIED, ERROR)")
	f.StringVar(&brandListBrandType, "brand-type", "", "Filter by brand type")
	f.StringVar(&brandListCompanyNameContains, "company-name-contains", "", "Filter by legal company name substring")
	f.StringVar(&brandListDisplayNameContains, "display-name-contains", "", "Filter by display name substring")
	brandCmd.AddCommand(brandListCmd)
}

var brandListCmd = &cobra.Command{
	Use:   "list",
	Short: "List 10DLC brands",
	Long: `Lists brands on the account.

This is a summary projection — 13 keys per brand, versus the 46 keys 'brand
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

		var filters []api.Filter
		if brandListCustomerProfileID != "" {
			filters = append(filters, api.Filter{Field: "customerProfileId", Op: api.OpEq, Value: brandListCustomerProfileID})
		}
		if brandListBrandID != "" {
			filters = append(filters, api.Filter{Field: "brandId", Op: api.OpEq, Value: brandListBrandID})
		}
		if brandListIdentityStatus != "" {
			filters = append(filters, api.Filter{Field: "brandIdentityStatus", Op: api.OpEq, Value: brandListIdentityStatus})
		}
		if brandListBrandType != "" {
			filters = append(filters, api.Filter{Field: "brandType", Op: api.OpEq, Value: brandListBrandType})
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
		return output.StdoutPlainList(format, plain, all)
	},
}
