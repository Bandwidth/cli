package tendlc

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() { campaignCmd.AddCommand(campaignGetCmd) }

var campaignGetCmd = &cobra.Command{
	Use:   "get <campaign-id>",
	Short: "Get a 10DLC campaign",
	Long: `Shows one campaign, including every field the summary projection in
'campaign list' omits — 45 keys versus list's 19.`,
	Example: `  band tendlc campaign get CEXMPL1 --plain`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		env, err := svc.GetCampaign(args[0])
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}
		obj, err := env.Object()
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, obj)
	},
}
