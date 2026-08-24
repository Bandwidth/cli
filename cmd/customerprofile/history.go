package customerprofile

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	historyLimit  int
	historyOffset int
	historyAll    bool
)

func init() {
	f := historyListCmd.Flags()
	f.IntVar(&historyLimit, "limit", 50, "Page size")
	f.IntVar(&historyOffset, "offset", 0, "Pagination offset")
	f.BoolVar(&historyAll, "all", false, "Fetch every page (cannot be combined with --offset)")

	historyCmd.AddCommand(historyListCmd)
	historyCmd.AddCommand(historyGetCmd)
	Cmd.AddCommand(historyCmd)
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Inspect a customer profile's version history",
}

var historyListCmd = &cobra.Command{
	Use:   "list <profile-id>",
	Short: "List a customer profile's versions",
	Long: `Lists every recorded version of a customer profile, newest first. Always returns an array.

Each entry nests the profile snapshot under "data" and audit fields under
"metadata" — the version number is at metadata.version, not top-level the way
it is on 'customer-profile get'. Observed metadata.operation values are
CREATED, UPDATED, and DELETED.`,
	Example: `  band customer-profile history list 3IIzIFnRRQBE3AMzPpMTNo --plain
  # [{"data":{"id":"...","name":"Acme"},"metadata":{"version":2,"operation":"UPDATED","userName":"...","createdDate":"..."}}, ...]`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if historyAll && cmd.Flags().Changed("offset") {
			return cmdutil.NewFlagError("--all fetches every page, so it cannot be combined with --offset")
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)

		if !historyAll {
			env, err := svc.History(args[0], historyLimit, historyOffset)
			if err != nil {
				return roleGateError(err)
			}
			items, err := env.List()
			if err != nil {
				return err
			}
			warnIfTruncated(cmd, env, historyOffset, len(items), "versions")
			return output.StdoutPlainList(format, plain, items)
		}

		var all []any
		err = api.ForEachPage(func(limit, offset int) (*api.Envelope, error) {
			return svc.History(args[0], limit, offset)
		}, historyLimit, func(batch []any) error {
			all = append(all, batch...)
			return nil
		})
		if err != nil {
			return roleGateError(err)
		}
		if all == nil {
			all = []any{}
		}
		return output.StdoutPlainList(format, plain, all)
	},
}

var historyGetCmd = &cobra.Command{
	Use:   "get <profile-id> <version>",
	Short: "Get one version of a customer profile",
	Long: `Shows a single historical version of a customer profile.

The response nests the profile snapshot under "data" and audit fields under
"metadata" — the version number is at metadata.version, not top-level the way
it is on 'customer-profile get'.

Separate from 'history list' so the --plain shape never depends on argument
count: list always returns an array, get always returns an object.`,
	Example: `  band customer-profile history get 3IIzIFnRRQBE3AMzPpMTNo 2 --plain
  # {"data":{"id":"...","name":"Acme"},"metadata":{"version":2,"operation":"UPDATED","userName":"...","createdDate":"..."}}`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		env, err := svc.HistoryVersion(args[0], args[1])
		if err != nil {
			return roleGateError(err)
		}
		obj, err := env.Object()
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, obj)
	},
}
