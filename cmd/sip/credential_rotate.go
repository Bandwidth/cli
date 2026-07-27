package sip

import (
	"github.com/spf13/cobra"

	sipsvc "github.com/Bandwidth/cli/internal/sip"
)

var credRotate credentialFlags

func init() {
	credentialCmd.AddCommand(credentialRotateCmd)
	credentialRotateCmd.Flags().StringVar(&credRotate.realm, "realm", "", "Realm ID, name, or FQDN (required)")
	addPasswordFlags(credentialRotateCmd, &credRotate)
	credentialRotateCmd.MarkFlagRequired("realm")
}

var credentialRotateCmd = &cobra.Command{
	Use:   "rotate <credential-id>",
	Short: "Rotate a SIP credential's password",
	Long: "Replaces a credential's digest hashes with ones derived from a new password. The credential ID is " +
		"unchanged, so peers referencing it keep working. This is the recovery path when a generated password " +
		"was lost. The username and application binding cannot be changed — delete and recreate for that.",
	Args:    cobra.ExactArgs(1),
	Example: `  band sip credential rotate 870880 --realm vapi --generate-password`,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		realm, err := svc.GetRealm(credRotate.realm)
		if err != nil {
			return faultExit(err)
		}
		existing, err := svc.GetCredential(realm.ID, args[0])
		if err != nil {
			return faultExit(err)
		}
		password, generated, err := readPassword(cmd, credRotate.stdin, credRotate.file, credRotate.generate)
		if err != nil {
			return err
		}
		hash1, hash1b := sipsvc.ComputeHashes(existing.Username, realm.Hostname, password)
		cred, err := svc.RotateCredential(realm.ID, existing.ID, hash1, hash1b)
		if err != nil {
			return faultExit(err)
		}
		return emitCredential(cmd, cred, password, generated)
	},
}
