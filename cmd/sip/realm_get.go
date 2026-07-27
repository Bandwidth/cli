package sip

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

func init() { realmCmd.AddCommand(realmGetCmd) }

var realmGetCmd = &cobra.Command{
	Use:     "get <realm-id-or-name>",
	Short:   "Get a SIP authentication realm",
	Long:    "Fetches one realm, including its generated FQDN. Accepts a realm ID, name, or FQDN.",
	Args:    cobra.ExactArgs(1),
	Example: `  band sip realm get vapi --plain`,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		realm, err := svc.GetRealm(args[0])
		if err != nil {
			return faultExit(err)
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return emit(format, plain, realm)
	},
}
