package tendlc

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	campaignPhoneNumbersLimit  int
	campaignPhoneNumbersOffset int
	campaignPhoneNumbersAll    bool
)

func init() {
	f := campaignPhoneNumbersCmd.Flags()
	f.IntVar(&campaignPhoneNumbersLimit, "limit", 50, "Page size")
	f.IntVar(&campaignPhoneNumbersOffset, "offset", 0, "Pagination offset")
	f.BoolVar(&campaignPhoneNumbersAll, "all", false, "Fetch every page (cannot be combined with --offset)")
	campaignCmd.AddCommand(campaignPhoneNumbersCmd)
}

var campaignPhoneNumbersCmd = &cobra.Command{
	Use:   "numbers <campaign-id>",
	Short: "List phone numbers assigned to a campaign",
	Long: `Lists the phone numbers assigned to a campaign, including numbers with
provisioning errors.`,
	Example: `  band tendlc campaign numbers CEXMPL1 --plain
  band tendlc campaign numbers CEXMPL1 --all --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if campaignPhoneNumbersAll && cmd.Flags().Changed("offset") {
			return cmdutil.NewFlagError("--all fetches every page, so it cannot be combined with --offset")
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)

		if !campaignPhoneNumbersAll {
			env, err := svc.CampaignPhoneNumbers(cmd.Context(), args[0], campaignPhoneNumbersLimit, campaignPhoneNumbersOffset)
			if err != nil {
				return roleGateError(err, "Campaign Management")
			}
			items, err := env.List()
			if err != nil {
				return err
			}
			warnIfTruncated(cmd, env, campaignPhoneNumbersOffset, len(items), "phone numbers")
			return output.StdoutPlainList(format, plain, items)
		}

		var all []any
		err = api.ForEachPage(func(limit, offset int) (*api.Envelope, error) {
			return svc.CampaignPhoneNumbers(cmd.Context(), args[0], limit, offset)
		}, campaignPhoneNumbersLimit, func(batch []any) error {
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
