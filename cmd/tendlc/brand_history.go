package tendlc

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	brandHistoryLimit  int
	brandHistoryOffset int
	brandHistoryAll    bool
)

func init() {
	f := brandHistoryCmd.Flags()
	f.IntVar(&brandHistoryLimit, "limit", 50, "Page size")
	f.IntVar(&brandHistoryOffset, "offset", 0, "Pagination offset")
	f.BoolVar(&brandHistoryAll, "all", false, "Fetch every page (cannot be combined with --offset)")
	brandCmd.AddCommand(brandHistoryCmd)
}

var brandHistoryCmd = &cobra.Command{
	Use:   "history <brand-id>",
	Short: "Show a brand's activity log",
	Long: `Lists a brand's activity log: free-text {createdDate, message} entries,
newest first. Always returns an array.

Unlike customer profiles, brands have no versioned snapshots and no
per-version fetch — this is the only history view for a brand.

Either the TCR brandId or the Bandwidth bandwidthId works as the positional.`,
	Example: `  band tendlc brand history BEXMPL6 --plain
  band tendlc brand history BEXMPL6 --all --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if brandHistoryAll && cmd.Flags().Changed("offset") {
			return cmdutil.NewFlagError("--all fetches every page, so it cannot be combined with --offset")
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)

		if !brandHistoryAll {
			env, err := svc.BrandHistory(args[0], brandHistoryLimit, brandHistoryOffset)
			if err != nil {
				return roleGateError(err, "Campaign Management")
			}
			items, err := env.List()
			if err != nil {
				return err
			}
			warnIfTruncated(cmd, env, brandHistoryOffset, len(items), "history entries")
			return output.StdoutPlainList(format, plain, items)
		}

		var all []any
		err = api.ForEachPage(func(limit, offset int) (*api.Envelope, error) {
			return svc.BrandHistory(args[0], limit, offset)
		}, brandHistoryLimit, func(batch []any) error {
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
