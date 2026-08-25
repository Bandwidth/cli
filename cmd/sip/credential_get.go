package sip

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

var credGetRealm string

func init() {
	credentialCmd.AddCommand(credentialGetCmd)
	credentialGetCmd.Flags().StringVar(&credGetRealm, "realm", "", "Realm ID, name, or FQDN (required)")
	credentialGetCmd.MarkFlagRequired("realm")
}

var credentialGetCmd = &cobra.Command{
	Use:     "get <credential-id>",
	Short:   "Get a SIP digest credential",
	Long:    "Fetches one SIP digest credential. Passwords are never returned — Bandwidth stores only MD5 hashes.",
	Args:    cobra.ExactArgs(1),
	Example: `  band sip credential get 870880 --realm vapi --plain`,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		realm, err := svc.GetRealm(cmd.Context(), credGetRealm)
		if err != nil {
			return faultExit(err)
		}
		cred, err := svc.GetCredential(cmd.Context(), realm.ID, args[0])
		if err != nil {
			return faultExit(err)
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return emit(format, plain, cred)
	},
}
