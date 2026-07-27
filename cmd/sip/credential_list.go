package sip

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

var credListRealm string

func init() {
	credentialCmd.AddCommand(credentialListCmd)
	credentialListCmd.Flags().StringVar(&credListRealm, "realm", "", "Realm ID, name, or FQDN (required)")
	credentialListCmd.MarkFlagRequired("realm")
}

var credentialListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List a realm's SIP credentials",
	Long:    "Lists the SIP digest credentials on a realm. Passwords are never returned — Bandwidth stores only MD5 hashes.",
	Example: `  band sip credential list --realm vapi --plain`,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		realm, err := svc.GetRealm(credListRealm)
		if err != nil {
			return faultExit(err)
		}
		creds, err := svc.ListCredentials(realm.ID)
		if err != nil {
			return faultExit(err)
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return emit(format, plain, creds)
	},
}
