package tendlc

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	campaignListLimit                int
	campaignListOffset               int
	campaignListAll                  bool
	campaignListBrandIDContains      string
	campaignListCampaignIDContains   string
	campaignListStatus               string
	campaignListVettingStatus        string
	campaignListCampaignNameContains string
)

func init() {
	f := campaignListCmd.Flags()
	f.IntVar(&campaignListLimit, "limit", 50, "Page size")
	f.IntVar(&campaignListOffset, "offset", 0, "Pagination offset")
	f.BoolVar(&campaignListAll, "all", false, "Fetch every page (cannot be combined with --offset)")
	f.StringVar(&campaignListBrandIDContains, "brand-id-contains", "",
		"Filter to campaigns whose owning brand ID contains this substring (e.g. BEXMPL1 also matches BEXMPL12); the API has no exact-match operator for this field")
	f.StringVar(&campaignListCampaignIDContains, "campaign-id-contains", "",
		"Filter by campaign ID substring (e.g. CEXMPL1 also matches CEXMPL12); the API has no exact-match operator for this field, use 'campaign get <id>' for a single campaign")
	f.StringVar(&campaignListStatus, "status", "", "Filter by campaign status, exact match")
	f.StringVar(&campaignListVettingStatus, "vetting-status", "", "Filter by vetting status, exact match")
	f.StringVar(&campaignListCampaignNameContains, "campaign-name-contains", "", "Filter by campaign name substring")
	campaignCmd.AddCommand(campaignListCmd)
}

var campaignListCmd = &cobra.Command{
	Use:   "list",
	Short: "List 10DLC campaigns",
	Long: `Lists campaigns on the account.

This is a summary projection — 19 keys per campaign, versus the 45 keys
'campaign get' returns. A field that is missing here may simply not be part
of the listing projection, not null on the campaign; use 'campaign get <id>'
for the full resource.

There is no --usecase filter: measured against production, usecase is
accepted under both eq and contains and silently ignored either way,
returning every campaign rather than filtering. It is also not among the
accepted query parameters for this endpoint in either spec file. Filter
client-side on the output of --all instead.`,
	Example: `  band tendlc campaign list --plain
  band tendlc campaign list --all --plain
  band tendlc campaign list --status REGISTERED --plain
  band tendlc campaign list --brand-id-contains BEXMPL1 --plain`,
	// No positional args: list takes only flags.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Detected via Changed so that an explicit --offset 0 also conflicts.
		if campaignListAll && cmd.Flags().Changed("offset") {
			return cmdutil.NewFlagError("--all fetches every page, so it cannot be combined with --offset")
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}

		// Measured against production (18 campaigns on the account): GET
		// /campaigns's operator support differs PER FIELD, and does not mirror
		// brand list's "everything needs contains" finding -- do not
		// "harmonize" the two commands.
		//
		//   status[eq]           -> 9 of 18   (works)
		//   vettingStatus[eq]    -> 9 of 18   (works)
		//   campaignName[contains] -> 8 of 18 (already implemented this way)
		//   brandId[eq]           -> 18 of 18 (ignored) | brandId[contains]      -> 1 of 18 (works)
		//   campaignId[eq]        -> 18 of 18 (ignored) | campaignId[contains]   -> 1 of 18 (works)
		//   usecase[eq] and usecase[contains] -> 18 of 18 (ignored under both operators;
		//     also absent from the accepted query parameters in both spec files -- see
		//     the "no --usecase filter" note in the Long text above. There is no flag
		//     for it at all, not merely a disabled one.)
		//
		// status and vettingStatus stay on eq because it genuinely filters here
		// -- do not switch them to contains "for consistency" with brand list;
		// that would just narrow a field that eq already narrows correctly.
		// brandId and campaignId, like brand list's brandId and
		// customerProfileId, only filter under contains, so their flags are
		// named for what they actually do (a substring match) rather than
		// implying an exact-match filter the API cannot perform.
		//
		// Unlike brand list's brandIdentityStatus/brandType, status and
		// vettingStatus need no client-side exact-match narrowing: eq is
		// already exact on the wire, so there is no contains-on-a-closed-enum
		// trap to close here.
		var filters []api.Filter
		if campaignListBrandIDContains != "" {
			filters = append(filters, api.Filter{Field: "brandId", Op: api.OpContains, Value: campaignListBrandIDContains})
		}
		if campaignListCampaignIDContains != "" {
			filters = append(filters, api.Filter{Field: "campaignId", Op: api.OpContains, Value: campaignListCampaignIDContains})
		}
		if campaignListStatus != "" {
			filters = append(filters, api.Filter{Field: "status", Op: api.OpEq, Value: campaignListStatus})
		}
		if campaignListVettingStatus != "" {
			filters = append(filters, api.Filter{Field: "vettingStatus", Op: api.OpEq, Value: campaignListVettingStatus})
		}
		if campaignListCampaignNameContains != "" {
			filters = append(filters, api.Filter{Field: "campaignName", Op: api.OpContains, Value: campaignListCampaignNameContains})
		}

		format, plain := cmdutil.OutputFlags(cmd)

		if !campaignListAll {
			env, err := svc.ListCampaigns(cmd.Context(), campaignListLimit, campaignListOffset, filters)
			if err != nil {
				return roleGateError(err, "Campaign Management")
			}
			items, err := env.List()
			if err != nil {
				return err
			}
			warnIfTruncated(cmd, env, campaignListOffset, len(items), "campaigns")
			return output.StdoutPlainList(format, plain, items)
		}

		var all []any
		err = api.ForEachPage(func(limit, offset int) (*api.Envelope, error) {
			return svc.ListCampaigns(cmd.Context(), limit, offset, filters)
		}, campaignListLimit, func(batch []any) error {
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
