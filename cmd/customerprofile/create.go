package customerprofile

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	cpsvc "github.com/Bandwidth/cli/internal/customerprofile"
	"github.com/Bandwidth/cli/internal/output"
)

var createOpts cpsvc.CreateOptions

func init() {
	f := createCmd.Flags()
	f.StringVar(&createOpts.Name, "name", "", "Profile name (required)")
	f.StringVar(&createOpts.Website, "website", "", "Business website URL")
	f.StringVar(&createOpts.ContactName, "contact-name", "", "Contact name (required if any other contact field is set)")
	f.StringVar(&createOpts.ContactPhone, "contact-phone", "", "Contact phone in E.164")
	f.StringVar(&createOpts.ContactEmail, "contact-email", "", "Contact email")
	f.StringVar(&createOpts.AddressID, "address-id", "", "Existing address ID to associate")
	Cmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a customer profile",
	Long: `Creates a customer profile.

A profile backs exactly one 10DLC brand, so create a new one for each brand
you intend to register — reusing a profile fails at brand creation.

This is a non-idempotent write: after an ambiguous failure, do not blindly
retry — list profiles and reconcile against what you submitted first.`,
	Example: `  band customer-profile create --name "Acme Corp" --plain

  band customer-profile create --name "Acme Corp" \
    --website https://acme.com \
    --contact-name "Ops Team" --contact-email ops@acme.com`,
	// No positional args: this is a non-idempotent create, so a stray
	// positional (e.g. a typo'd second word meant for another flag) must be
	// rejected rather than silently ignored and creating an unintended profile.
	Args: cobra.NoArgs,
	// Required-ness is enforced in RunE, not via MarkFlagRequired: cobra
	// rejects before RunE, which reports one flag at a time and would block a
	// future interactive prompt from filling them in.
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cpsvc.ValidateCreate(createOpts); err != nil {
			return err
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		env, err := svc.Create(cmd.Context(), cpsvc.BuildCreateRequest(createOpts))
		if err != nil {
			return roleGateError(err)
		}
		obj, err := env.Object()
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, obj)
	},
}
