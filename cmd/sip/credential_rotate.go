package sip

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
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
			if generated {
				// The PUT may have replaced the hashes server-side even though
				// this call reports failure (e.g. a decode error after a
				// successful write). The generated password was never printed,
				// so a working peer's credential may now be silently dead with
				// an unrecoverable password — that must not exit as a generic,
				// retryable failure.
				return &cmdutil.SecretUnavailableError{Message: fmt.Sprintf(
					"the write may have been applied but the generated password was not printed and cannot be recovered — rotate the credential again: %v", err)}
			}
			return faultExit(err)
		}
		return emitCredential(cmd, cred, password, generated)
	},
}
