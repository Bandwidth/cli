package shortcode

import "github.com/spf13/cobra"

// Cmd is the `band shortcode` parent command.
var Cmd = &cobra.Command{
	Use:   "shortcode",
	Short: "View short code registrations and carrier status",
	Long: `View short codes registered to your account and their per-carrier activation status.

Short codes are provisioned through carrier agreements outside the API.
These commands are read-only — use them to verify a short code is active
before sending messages through it.`,
}
