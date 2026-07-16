package account

import "github.com/spf13/cobra"

// Cmd is the `band account` parent command.
var Cmd = &cobra.Command{
	Use:   "account",
	Short: "Manage Bandwidth account registration",
	Long:  "Register and provision Bandwidth accounts. These commands handle account onboarding and do not require an existing account to be selected.",
}
