package sip

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

var realmUpdateDefault bool

func init() {
	realmCmd.AddCommand(realmUpdateCmd)
	realmUpdateCmd.Flags().BoolVar(&realmUpdateDefault, "default", false, "Make this realm the account default (only true is supported by the API)")
	realmUpdateCmd.MarkFlagRequired("default")
}

var realmUpdateCmd = &cobra.Command{
	Use:   "update <realm-id-or-name>",
	Short: "Make a realm the account default",
	Long: "Sets a realm as the account default. This exists so a default realm can be torn down: " +
		"the API refuses to delete the default realm, and 'default' can only be set to true, so another " +
		"realm must be promoted first.",
	Args:    cobra.ExactArgs(1),
	Example: `  band sip realm update backup-realm --default=true`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !realmUpdateDefault {
			return fmt.Errorf("--default=false is not supported by the API; promote a different realm instead")
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		realm, err := svc.SetRealmDefault(args[0])
		if err != nil {
			return faultExit(err)
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return emit(format, plain, realm)
	},
}
