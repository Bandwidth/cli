package tendlc

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	brandDeleteConfirm bool
	brandDeleteWait    bool
	brandDeleteTimeout int
)

func init() {
	f := brandDeleteCmd.Flags()
	f.BoolVar(&brandDeleteConfirm, "confirm", false, "Required. Confirms the brand should be deleted.")
	f.BoolVar(&brandDeleteWait, "wait", false, "Block until the brand is gone")
	f.IntVar(&brandDeleteTimeout, "timeout", 300, "Seconds to wait when --wait is set")
	brandCmd.AddCommand(brandDeleteCmd)
}

var brandDeleteCmd = &cobra.Command{
	Use:   "delete <brand-id>",
	Short: "Permanently delete a 10DLC brand",
	Long: `Permanently deletes a 10DLC brand. This cannot be undone.

Deleting a brand deletes the brand in TCR for direct accounts, and requires
every campaign on the brand to be deactivated first — the API rejects the
delete otherwise.

The endpoint docs say this cascades to delete the backing customer profile.
Measured on production: it does not. After deleting two test brands, both
backing profiles remained retrievable with softDeleted:false. The profile is
NOT deleted by this command — if you no longer need it, remove it separately
with 'band customer-profile delete <id>'. A profile backs exactly one brand,
so an orphaned one left behind after a brand delete cannot be reused.

Requires --confirm. With --wait, this polls until the brand is gone: a 404 on
the follow-up read IS success here, the only place in this command set where
that is true.`,
	Example: `  band tendlc brand delete BGJR2BA --confirm --plain
  band tendlc brand delete BGJR2BA --confirm --wait --timeout 60 --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfirm(brandDeleteConfirm,
			"this permanently deletes brand "+args[0]+". It cannot be undone, it deletes the brand in TCR "+
				"for direct accounts, and it requires every campaign on the brand to be deactivated first. "+
				"It does NOT delete the associated customer profile (measured against production — the "+
				"documented cascade does not happen); remove that separately with "+
				"'band customer-profile delete <id>' if you no longer need it. Pass --confirm to proceed."); err != nil {
			return err
		}

		svc, err := service(cmd)
		if err != nil {
			return err
		}
		if err := svc.DeleteBrand(args[0]); err != nil {
			return roleGateError(err, "Campaign Management")
		}

		receipt := map[string]any{
			"id":      args[0],
			"deleted": true,
			"status":  "accepted",
		}

		if !brandDeleteWait {
			format, _ := cmdutil.OutputFlags(cmd)
			return output.Stdout(format, receipt)
		}

		// GoneIsDone: true — a 404 on the follow-up read means the delete has
		// actually taken effect. This is the only poll in the whole tendlc
		// command set that treats 404 as success rather than "not ready yet",
		// which is exactly why the field is explicit on pollTarget instead of
		// inferred from the noun.
		target := pollTarget{
			Noun:       "brand",
			Fetch:      fetchBrand(svc, args[0]),
			GoneIsDone: true,
		}
		return awaitTerminal(cmd, target, receipt, time.Duration(brandDeleteTimeout)*time.Second, brandCreatePollInterval)
	},
}
