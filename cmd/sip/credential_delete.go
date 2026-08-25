package sip

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

var credDeleteRealm string

func init() {
	credentialCmd.AddCommand(credentialDeleteCmd)
	credentialDeleteCmd.Flags().StringVar(&credDeleteRealm, "realm", "", "Realm ID, name, or FQDN (required)")
	credentialDeleteCmd.MarkFlagRequired("realm")
}

var credentialDeleteCmd = &cobra.Command{
	Use:     "delete <credential-id>",
	Short:   "Delete a SIP digest credential",
	Long:    "Deletes a SIP digest credential from a realm.",
	Args:    cobra.ExactArgs(1),
	Example: `  band sip credential delete 870880 --realm vapi --plain`,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		realm, err := svc.GetRealm(cmd.Context(), credDeleteRealm)
		if err != nil {
			return faultExit(err)
		}
		if err := svc.DeleteCredential(cmd.Context(), realm.ID, args[0]); err != nil {
			return faultExit(err)
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return emit(format, plain, map[string]interface{}{
			"id":      args[0],
			"realmId": realm.ID,
			"deleted": true,
		})
	},
}
