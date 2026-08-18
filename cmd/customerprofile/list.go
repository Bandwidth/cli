package customerprofile

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	listLimit        int
	listOffset       int
	listAll          bool
	listNameContains string
)

func init() {
	f := listCmd.Flags()
	f.IntVar(&listLimit, "limit", 50, "Page size")
	f.IntVar(&listOffset, "offset", 0, "Pagination offset")
	f.BoolVar(&listAll, "all", false, "Fetch every page (cannot be combined with --offset)")
	f.StringVar(&listNameContains, "name-contains", "", "Filter by profile name substring")
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List customer profiles",
	Long:  "Lists customer profiles on the account. Soft-deleted profiles are excluded from this listing but remain retrievable by ID with 'customer-profile get'.",
	Example: `  band customer-profile list --plain
  band customer-profile list --all --plain
  band customer-profile list --name-contains Acme --plain`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Detected via Changed so that an explicit --offset 0 also conflicts.
		if listAll && cmd.Flags().Changed("offset") {
			return cmdutil.NewFlagError("--all fetches every page, so it cannot be combined with --offset")
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}

		var filters []api.Filter
		if listNameContains != "" {
			filters = append(filters, api.Filter{Field: "name", Op: api.OpContains, Value: listNameContains})
		}

		format, plain := cmdutil.OutputFlags(cmd)

		if !listAll {
			env, err := svc.List(listLimit, listOffset, filters)
			if err != nil {
				return err
			}
			items, err := env.List()
			if err != nil {
				return err
			}
			warnIfTruncated(cmd, env, len(items))
			return output.StdoutPlainList(format, plain, items)
		}

		var all []any
		err = api.ForEachPage(func(limit, offset int) (*api.Envelope, error) {
			return svc.List(limit, offset, filters)
		}, listLimit, func(batch []any) error {
			all = append(all, batch...)
			return nil
		})
		if err != nil {
			return err
		}
		if all == nil {
			all = []any{}
		}
		return output.StdoutPlainList(format, plain, all)
	},
}

// warnIfTruncated tells the caller on stderr when more records exist. stdout
// stays clean so a pipeline sees only data.
func warnIfTruncated(cmd *cobra.Command, env *api.Envelope, returned int) {
	if env.Page != nil && env.Page.Truncated(listOffset+returned) {
		cmd.PrintErrf("showing %d of %d profiles; pass --all to fetch every page\n",
			returned, env.Page.TotalElements)
	}
}
