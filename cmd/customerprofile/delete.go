package customerprofile

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	cpsvc "github.com/Bandwidth/cli/internal/customerprofile"
	"github.com/Bandwidth/cli/internal/output"
)

var deleteConfirm bool

func init() {
	deleteCmd.Flags().BoolVar(&deleteConfirm, "confirm", false,
		"Required. Confirms the profile should be deleted.")
	Cmd.AddCommand(deleteCmd)
	Cmd.AddCommand(restoreCmd)
}

var deleteCmd = &cobra.Command{
	Use:   "delete <profile-id>",
	Short: "Soft-delete a customer profile",
	Long: `Soft-deletes a customer profile.

The record is removed from listings but remains retrievable by ID with
'customer-profile get', reporting softDeleted: true, and can be brought back
with 'customer-profile restore'.

Requires --confirm. That is a flag rather than a prompt, so scripts, agents,
and humans all get the same contract regardless of whether a terminal is
attached.`,
	Example: `  band customer-profile delete 3IIzIFnRRQBE3AMzPpMTNo --confirm --plain`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfirm(deleteConfirm,
			"this soft-deletes customer profile "+args[0]+
				", removing it from listings; pass --confirm to proceed "+
				"(restore it later with 'band customer-profile restore "+args[0]+"')"); err != nil {
			return err
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		if err := svc.Delete(cmd.Context(), args[0]); err != nil {
			return roleGateError(err)
		}
		format, plain := cmdutil.OutputFlags(cmd)
		// A 204 is a completed delete, not an async acceptance, so the receipt
		// says "deleted" rather than "accepted".
		return output.StdoutAuto(format, plain, map[string]any{
			"id":      args[0],
			"deleted": true,
			"restore": "band customer-profile restore " + args[0],
		})
	},
}

var restoreCmd = &cobra.Command{
	Use:   "restore <profile-id>",
	Short: "Restore a soft-deleted customer profile",
	Long: `Restores a soft-deleted customer profile.

Sends softDeleted: false. Note the published API docs describe restoring with
{"deleted": false} — that form returns 404 "Customer profile not found" even
though the record is retrievable. Reported to the API team.

No --confirm needed: restoring is not destructive.`,
	Example: `  band customer-profile restore 3IIzIFnRRQBE3AMzPpMTNo --plain`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		env, err := svc.Get(cmd.Context(), args[0])
		if err != nil {
			return roleGateError(err)
		}
		current, err := env.Object()
		if err != nil {
			return err
		}
		body, err := cpsvc.BuildRestoreRequest(current)
		if err != nil {
			return err
		}
		restored, err := svc.Update(cmd.Context(), args[0], body)
		if err != nil {
			return roleGateError(conflictHint(err))
		}
		obj, err := restored.Object()
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, obj)
	},
}
