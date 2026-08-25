package sip

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

var (
	realmUpdateDefault     bool
	realmUpdateDescription string
)

func init() {
	realmCmd.AddCommand(realmUpdateCmd)
	realmUpdateCmd.Flags().BoolVar(&realmUpdateDefault, "default", false, "Make this realm the account default (only true is supported by the API)")
	realmUpdateCmd.Flags().StringVar(&realmUpdateDescription, "description", "", "Set the realm's description")
}

var realmUpdateCmd = &cobra.Command{
	Use:   "update <realm-id-or-name>",
	Short: "Update a SIP realm's default flag or description",
	Long: "Updates a realm. Two fields are updatable: --default=true promotes the realm to the account " +
		"default, and --description replaces its description. Promotion exists so a default realm can be " +
		"torn down: the API refuses to delete the default realm, and 'default' can only be set to true, so " +
		"another realm must be promoted first. --description is the remediation 'sip realm create " +
		"--if-not-exists' names when an existing realm's description differs from what was requested. " +
		"Omitted fields are preserved (the update is read-modify-write over the API's full-replace PUT).",
	Args: cobra.ExactArgs(1),
	Example: `  # Promote a realm to account default
  band sip realm update backup-realm --default=true

  # Change only the description (default flag is preserved)
  band sip realm update vapi --description "Vapi production trunk"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		defaultSet := cmd.Flags().Changed("default")
		descSet := cmd.Flags().Changed("description")
		// --default=false is still rejected outright: the API cannot demote a
		// realm, and silently ignoring the flag would misreport success.
		if defaultSet && !realmUpdateDefault {
			return fmt.Errorf("--default=false is not supported by the API; promote a different realm instead")
		}
		if !defaultSet && !descSet {
			return fmt.Errorf("specify at least one of --default=true or --description")
		}

		svc, err := service(cmd)
		if err != nil {
			return err
		}
		var desc *string
		if descSet {
			desc = &realmUpdateDescription
		}
		realm, err := svc.UpdateRealm(cmd.Context(), args[0], defaultSet && realmUpdateDefault, desc)
		if err != nil {
			return faultExit(err)
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return emit(format, plain, realm)
	},
}
