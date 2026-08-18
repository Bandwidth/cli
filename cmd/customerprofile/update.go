package customerprofile

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	cpsvc "github.com/Bandwidth/cli/internal/customerprofile"
	"github.com/Bandwidth/cli/internal/output"
)

var updateOpts cpsvc.UpdateOptions

// updateFieldFlags are the flags that carry a field value, in CLI naming.
// BuildUpdateRequest keys its changed-map on exactly these names.
var updateFieldFlags = []string{"name", "website", "contact-name", "contact-phone", "contact-email", "address-id"}

func init() {
	f := updateCmd.Flags()
	f.StringVar(&updateOpts.Name, "name", "", "Profile name")
	f.StringVar(&updateOpts.Website, "website", "", "Business website URL")
	f.StringVar(&updateOpts.ContactName, "contact-name", "", "Contact name")
	f.StringVar(&updateOpts.ContactPhone, "contact-phone", "", "Contact phone in E.164")
	f.StringVar(&updateOpts.ContactEmail, "contact-email", "", "Contact email")
	f.StringVar(&updateOpts.AddressID, "address-id", "", "Address ID to associate")
	Cmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update <profile-id>",
	Short: "Update a customer profile",
	Long: `Updates a customer profile.

The API replaces the whole record on update, so this command reads the profile
first and sends it back with your changes applied. Fields you do not pass are
preserved. Passing a flag with an empty value clears that field.

Because the read and the write are two requests, a concurrent edit between them
is rejected by the API's version check — the command exits 4 and you can retry.`,
	Example: `  band customer-profile update 3IIzIFnRRQBE3AMzPpMTNo --name "Acme Corp" --plain
  band customer-profile update 3IIzIFnRRQBE3AMzPpMTNo --website "" --plain   # clear the website`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		changed := map[string]bool{}
		any := false
		for _, name := range updateFieldFlags {
			if cmd.Flags().Changed(name) {
				changed[name] = true
				any = true
			}
		}
		if !any {
			return cmdutil.NewFlagError(
				"nothing to update — pass at least one of " + flagList(updateFieldFlags))
		}

		svc, err := service(cmd)
		if err != nil {
			return err
		}

		env, err := svc.Get(args[0])
		if err != nil {
			return err
		}
		current, err := env.Object()
		if err != nil {
			return err
		}

		body, err := cpsvc.BuildUpdateRequest(current, updateOpts, changed)
		if err != nil {
			return err
		}

		updated, err := svc.Update(args[0], body)
		if err != nil {
			return conflictHint(err)
		}
		obj, err := updated.Object()
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, obj)
	},
}

// conflictHint turns the API's version conflict into an actionable exit 4.
// The same 409 is returned when version is stale and when it is missing, so
// the message covers the case the caller can actually act on.
func conflictHint(err error) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 409 {
		return &cmdutil.ConflictError{
			Message: "this profile was modified by someone else while the update was in flight; retry the command",
			Cause:   err,
		}
	}
	return err
}

func flagList(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += "--" + n
	}
	return out
}
