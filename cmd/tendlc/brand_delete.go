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

Requires --confirm. The DELETE only ACCEPTS the request — production takes
roughly 40 seconds to actually remove the brand, so the receipt's "deleted"
field is false until that is confirmed. Without --wait, it stays false;
confirm manually with 'band tendlc brand get <id>' (a 404 means it is gone).
With --wait, this polls until the brand is gone — a 404 on the follow-up read
IS success here, the only place in this command set where that is true — and
only then does "deleted" flip to true. A --wait timeout (exit 5) prints the
same unconfirmed receipt as no-wait; it never claims deleted:true merely
because --wait gave up waiting.`,
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

		// The DELETE returning is only an accept, not a completion — measured
		// against production, it takes roughly 40 seconds before the brand
		// actually disappears from 'brand list'/'brand get'. So "deleted"
		// starts false and stays false unless and until a 404 on the
		// follow-up read actually proves it happened: not on the strength of
		// the 202 alone, and not merely because --wait gave up waiting.
		receipt := map[string]any{
			"id":      args[0],
			"deleted": false,
			"status":  "accepted",
			"note": "delete accepted but not yet confirmed: production takes roughly 40s to actually " +
				"remove the brand, so it may still appear in 'brand list' or 'brand get' until then. " +
				"Confirm with 'band tendlc brand get " + args[0] + "' — a 404 means it is gone.",
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
		//
		// confirmedFetch wraps fetchBrand so the receipt only ever claims
		// deleted:true at the exact moment a 404 proves it. awaitTerminal
		// prints this same receipt map on BOTH the confirmed-success path and
		// the --timeout path, so if the deadline arrives before this ever
		// fires, deleted is still false and the note above is still there —
		// the timeout receipt (exit 5) never contradicts its own exit code.
		fetch := fetchBrand(svc, args[0])
		confirmedFetch := func() (map[string]any, bool, error) {
			obj, found, ferr := fetch()
			if ferr == nil && !found {
				receipt["deleted"] = true
				delete(receipt, "note")
			}
			return obj, found, ferr
		}

		target := pollTarget{
			Noun:       "brand",
			Fetch:      confirmedFetch,
			GoneIsDone: true,
		}
		return awaitTerminal(cmd, target, receipt, time.Duration(brandDeleteTimeout)*time.Second, brandCreatePollInterval)
	},
}
