package tendlc

import "github.com/spf13/cobra"

// Cmd is the `band tendlc` parent command.
var Cmd = &cobra.Command{
	Use:   "tendlc",
	Short: "10DLC campaign and number registration status",
	Long: `View 10DLC campaigns, brands, and phone number registration status.

Requires the Campaign Management role and the Registration Center feature on your
account. If you get a 403 error, contact your Bandwidth account manager to enable access.`,
}
