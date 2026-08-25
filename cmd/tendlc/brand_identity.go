package tendlc

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var brandReverifyConfirm bool

func init() {
	f := brandReverifyCmd.Flags()
	f.BoolVar(&brandReverifyConfirm, "confirm", false, "Required. Confirms the $4 reverification fee.")
	brandCmd.AddCommand(brandReverifyCmd)
	brandCmd.AddCommand(brandResend2FACmd)
}

var brandReverifyCmd = &cobra.Command{
	Use:   "reverify <brand-id>",
	Short: "Resubmit a brand for identity verification",
	Long: `Resubmits a brand for identity verification.

This incurs a $4 fee and resets brandIdentityStatus toward re-registration.
Production documents this as REGISTERING, but the field reads back as
UNVERIFIED until TCR responds — see 'band tendlc brand get <brand-id>'. The
endpoint returns 204 with no body — there is no ID or resource to poll, so
there is no --wait here. The brand's own status is the signal.

Requires --confirm.`,
	Example: `  band tendlc brand reverify BEXMPL6 --confirm --plain`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfirm(brandReverifyConfirm,
			"reverifying brand "+args[0]+" incurs a $4 fee and resets brandIdentityStatus toward "+
				"re-registration; it reads back as UNVERIFIED until TCR responds. Pass --confirm to proceed."); err != nil {
			return err
		}

		svc, err := service(cmd)
		if err != nil {
			return err
		}
		if err := svc.ReverifyBrand(cmd.Context(), args[0]); err != nil {
			return roleGateError(err, "Campaign Management")
		}

		receipt := map[string]any{
			"id":                      args[0],
			"reverificationRequested": true,
			"status":                  "accepted",
			"check":                   "band tendlc brand get " + args[0],
		}
		format, _ := cmdutil.OutputFlags(cmd)
		return output.Stdout(format, receipt)
	},
}

var brandResend2FACmd = &cobra.Command{
	Use:   "resend-2fa <brand-id>",
	Short: "Re-send the Business Authentication 2FA email",
	Long: `Re-sends the Business Authentication (Auth+) 2FA email to a brand's
business contact.

This applies to Business Authentication on PUBLIC_PROFIT brands. The
business contact has 30 days from the original request to complete
verification before the brand goes UNVERIFIED.

Re-sending an email is neither destructive nor billable, so unlike most
writes in this command set, this does not require --confirm.`,
	Example: `  band tendlc brand resend-2fa BEXMPL6 --plain`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		if err := svc.Resend2FA(cmd.Context(), args[0]); err != nil {
			return roleGateError(err, "Campaign Management")
		}

		receipt := map[string]any{
			"id":          args[0],
			"emailResent": true,
		}
		format, _ := cmdutil.OutputFlags(cmd)
		return output.Stdout(format, receipt)
	},
}
