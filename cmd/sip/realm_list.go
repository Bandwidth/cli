package sip

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

func init() { realmCmd.AddCommand(realmListCmd) }

var realmListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List SIP authentication realms",
	Example: `  band sip realm list --plain`,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		realms, err := svc.ListRealms(cmd.Context())
		if err != nil {
			return faultExit(err)
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return emit(format, plain, realms)
	},
}
