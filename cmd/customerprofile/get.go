package customerprofile

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() { Cmd.AddCommand(getCmd) }

var getCmd = &cobra.Command{
	Use:     "get <profile-id>",
	Short:   "Get a customer profile",
	Long:    "Shows one customer profile. Soft-deleted profiles are still retrievable here and report softDeleted: true.",
	Example: `  band customer-profile get 3IIzIFnRRQBE3AMzPpMTNo --plain`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		env, err := svc.Get(args[0])
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
