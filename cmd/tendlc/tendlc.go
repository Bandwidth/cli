package tendlc

import "github.com/spf13/cobra"

// Cmd is the `band tendlc` parent command.
var Cmd = &cobra.Command{
	Use:   "tendlc",
	Short: "10DLC campaign and number registration status",
	Long: `View 10DLC campaigns, brands, and phone number registration status.

Requires the Campaign Management role and the Registration Center feature on your
account. If you get a 403 error, contact your Bandwidth account manager to enable access.`,
	Args: cobra.NoArgs,
	// A trivial RunE is required, not decorative: cobra's execute() checks
	// Runnable() (Run/RunE set) BEFORE it ever calls ValidateArgs, so a
	// parent with no RunE always short-circuits to flag.ErrHelp regardless
	// of Args -- Args: cobra.NoArgs above would silently never run. Calling
	// cmd.Help() here keeps the existing bare-invocation behavior (print
	// help, exit 0) while letting a stray positional -- e.g. a deleted
	// command's name -- reach NoArgs and fail with a non-zero exit instead
	// of being silently swallowed as if it were a help request.
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}
