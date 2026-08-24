package tendlc

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() { brandCmd.AddCommand(brandGetCmd) }

var brandGetCmd = &cobra.Command{
	Use:   "get <brand-id>",
	Short: "Get a 10DLC brand",
	Long: `Shows one brand, including every field the summary projection in
'brand list' omits.

Brands have two IDs: bandwidthId exists immediately, while brandId is assigned
by TCR and is null until registration completes. Either identifier works
here — pass whichever one you have.`,
	Example: `  band tendlc brand get BGJR2BA --plain
  band tendlc brand get WET8JUY8H0 --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		env, err := svc.GetBrand(args[0])
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
