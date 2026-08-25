package tendlc

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	campaignHistoryLimit  int
	campaignHistoryOffset int
	campaignHistoryAll    bool
)

func init() {
	f := campaignHistoryCmd.Flags()
	f.IntVar(&campaignHistoryLimit, "limit", 50, "Page size")
	f.IntVar(&campaignHistoryOffset, "offset", 0, "Pagination offset")
	f.BoolVar(&campaignHistoryAll, "all", false, "Fetch every page (cannot be combined with --offset)")
	campaignCmd.AddCommand(campaignHistoryCmd)
}

var campaignHistoryCmd = &cobra.Command{
	Use:   "history <campaign-id>",
	Short: "Show a campaign's activity log",
	Long: `Lists a campaign's activity log: free-text {createdDate, message} entries,
newest first. Always returns an array.

Unlike customer profiles, campaigns have no versioned snapshots and no
per-entry fetch — this is the only history view for a campaign; there is
deliberately no 'campaign history get'.

An update pushes the change to TCR, and the campaign is then re-imported
from TCR and re-enters carrier review. That round trip is why a single
'campaign update' can produce several history entries in quick succession
rather than one.`,
	Example: `  band tendlc campaign history CEXMPL1 --plain
  band tendlc campaign history CEXMPL1 --all --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if campaignHistoryAll && cmd.Flags().Changed("offset") {
			return cmdutil.NewFlagError("--all fetches every page, so it cannot be combined with --offset")
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)

		if !campaignHistoryAll {
			env, err := svc.CampaignHistory(cmd.Context(), args[0], campaignHistoryLimit, campaignHistoryOffset)
			if err != nil {
				return roleGateError(err, "Campaign Management")
			}
			items, err := env.List()
			if err != nil {
				return err
			}
			warnIfTruncated(cmd, env, campaignHistoryOffset, len(items), "history entries")
			return output.StdoutPlainList(format, plain, items)
		}

		var all []any
		err = api.ForEachPage(func(limit, offset int) (*api.Envelope, error) {
			return svc.CampaignHistory(cmd.Context(), args[0], limit, offset)
		}, campaignHistoryLimit, func(batch []any) error {
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
