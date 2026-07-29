package sip

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	sipsvc "github.com/Bandwidth/cli/internal/sip"
)

func init() { Cmd.AddCommand(statusCmd) }

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check whether this account can use SIP provisioning",
	Long: "Probes the SIP API to resolve the 'unknown' capability reported by 'band auth status'. " +
		"SIP provisioning requires both the SIP Credentials role and account-level configuration; " +
		"only the probe can confirm the latter. The result is not cached — 'band auth status' stays offline.",
	Example: `  band sip status --plain`,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)

		if _, err := svc.ListRealms(); err != nil {
			var fault *sipsvc.APIFault
			if errors.As(err, &fault) && fault.Code == "33004" {
				// A successful probe reporting a negative fact: exit 0.
				return emit(format, plain, map[string]string{
					"status": "unavailable",
					"reason": "account_not_enabled",
				})
			}
			return faultExit(err)
		}
		return emit(format, plain, map[string]string{
			"status": "available",
			"reason": "probe_succeeded",
		})
	},
}
