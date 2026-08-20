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
	campaignListBrandID              string
	campaignListCampaignID           string
	campaignListStatus               string
	campaignListUsecase              string
	campaignListVettingStatus        string
	campaignListCampaignNameContains string
)

func init() {
	f := campaignListCmd.Flags()
	f.IntVar(&campaignListLimit, "limit", 50, "Page size")
	f.IntVar(&campaignListOffset, "offset", 0, "Pagination offset")
	f.BoolVar(&campaignListAll, "all", false, "Fetch every page (cannot be combined with --offset)")
	f.StringVar(&campaignListBrandID, "brand-id", "", "Filter by the brand that owns the campaign")
	f.StringVar(&campaignListCampaignID, "campaign-id", "", "Filter by campaign ID")
	f.StringVar(&campaignListStatus, "status", "", "Filter by campaign status")
	f.StringVar(&campaignListUsecase, "usecase", "", "Filter by campaign usecase")
	f.StringVar(&campaignListVettingStatus, "vetting-status", "", "Filter by vetting status")
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
for the full resource.`,
	Example: `  band tendlc campaign list --plain
  band tendlc campaign list --all --plain
  band tendlc campaign list --status REGISTERED --plain
  band tendlc campaign list --brand-id BEXMPL1 --plain`,
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

		var filters []api.Filter
		if campaignListBrandID != "" {
			filters = append(filters, api.Filter{Field: "brandId", Op: api.OpEq, Value: campaignListBrandID})
		}
		if campaignListCampaignID != "" {
			filters = append(filters, api.Filter{Field: "campaignId", Op: api.OpEq, Value: campaignListCampaignID})
		}
		if campaignListStatus != "" {
			filters = append(filters, api.Filter{Field: "status", Op: api.OpEq, Value: campaignListStatus})
		}
		if campaignListUsecase != "" {
			filters = append(filters, api.Filter{Field: "usecase", Op: api.OpEq, Value: campaignListUsecase})
		}
		if campaignListVettingStatus != "" {
			filters = append(filters, api.Filter{Field: "vettingStatus", Op: api.OpEq, Value: campaignListVettingStatus})
		}
		if campaignListCampaignNameContains != "" {
			filters = append(filters, api.Filter{Field: "campaignName", Op: api.OpContains, Value: campaignListCampaignNameContains})
		}

		format, plain := cmdutil.OutputFlags(cmd)

		if !campaignListAll {
			env, err := svc.ListCampaigns(campaignListLimit, campaignListOffset, filters)
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
			return svc.ListCampaigns(limit, offset, filters)
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
