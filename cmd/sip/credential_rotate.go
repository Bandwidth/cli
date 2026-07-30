package sip

import (
	"errors"
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
			// The PUT may have replaced the hashes server-side even though this
			// call reports failure (e.g. a decode error after a successful
			// write). The generated password was never printed, so a working
			// peer's credential may now be silently dead with an unrecoverable
			// password — that must not exit as a generic, retryable failure.
			//
			// An *APIFault is excluded: the server parsed and rejected the
			// request, so the hashes were NOT replaced. Reporting exit 8 there
			// sends an agent down the unrecoverable-secret path for what is
			// often just a 429 it should retry.
			var fault *sipsvc.APIFault
			if generated && !errors.As(err, &fault) {
				return &cmdutil.SecretUnavailableError{Message: fmt.Sprintf(
					"the write may have been applied but the generated password was not printed and cannot be recovered — rotate the credential again: %v", err)}
			}
			return faultExit(err)
		}
		// The PUT has committed: a working peer's hashes are already replaced. A
		// stdout write failure here is the worst case in this command — the peer
		// is broken AND the only copy of the password that would fix it is gone.
		// That is exit 8 (rotate again), never the generic 1 a write error maps
		// to. Unlike create, the credential ID is known, so the recovery command
		// is named in full. With a caller-supplied password nothing is lost, so
		// the write error is returned unchanged.
		if err := emitCredential(cmd, cred, password, generated); err != nil {
			if generated {
				return &cmdutil.SecretUnavailableError{Message: fmt.Sprintf(
					"credential %s was rotated but the generated password could not be written to stdout and cannot be recovered — the SIP peer using it cannot authenticate until you rotate it again: band sip credential rotate %s --realm %s --generate-password: %v",
					existing.ID, existing.ID, realm.Name, err)}
			}
			return err
		}
		return nil
	},
}
