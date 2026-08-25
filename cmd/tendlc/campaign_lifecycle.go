package tendlc

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	tendlcsvc "github.com/Bandwidth/cli/internal/tendlc"
)

var (
	campaignDeactivateConfirm bool
	campaignDeactivateWait    bool
	campaignDeactivateTimeout int

	campaignNudgeIntent      string
	campaignNudgeDescription string
)

func init() {
	df := campaignDeactivateCmd.Flags()
	df.BoolVar(&campaignDeactivateConfirm, "confirm", false, "Required. Confirms the campaign should be deactivated.")
	df.BoolVar(&campaignDeactivateWait, "wait", false, "Block until the campaign is gone")
	df.IntVar(&campaignDeactivateTimeout, "timeout", 300, "Seconds to wait when --wait is set")
	campaignCmd.AddCommand(campaignDeactivateCmd)

	nf := campaignNudgeCmd.Flags()
	nf.StringVar(&campaignNudgeIntent, "intent", "",
		"Required. Re-evaluation intent: "+strings.Join(tendlcsvc.NudgeIntents, " or "))
	nf.StringVar(&campaignNudgeDescription, "description", "",
		"Optional context for the re-evaluation request (max 1024 characters, enforced server-side)")
	campaignCmd.AddCommand(campaignNudgeCmd)
}

var campaignDeactivateCmd = &cobra.Command{
	Use:   "deactivate <campaign-id>",
	Short: "Permanently deactivate a 10DLC campaign",
	Long: `Permanently deactivates a 10DLC campaign. This cannot be undone.

Deactivating a campaign ends message delivery for it and removes it from
Bandwidth; any phone numbers assigned to the campaign stop working for
messaging.

Requires --confirm. The delete only ACCEPTS the request — deactivation is
asynchronous, so the receipt's "deactivated" field is false until that is
confirmed. Without --wait, it stays false; confirm manually with
'band tendlc campaign get <campaign-id>' (a 404 means it is gone). With
--wait, this polls until the campaign is gone — a 404 on the follow-up read
IS success here, the only place in the campaign command set where that is
true — and only then does "deactivated" flip to true. A --wait timeout
(exit 5) prints the same unconfirmed receipt as no-wait; it never claims
deactivated:true merely because --wait gave up waiting.`,
	Example: `  band tendlc campaign deactivate CEXMPL1 --confirm --plain
  band tendlc campaign deactivate CEXMPL1 --confirm --wait --timeout 60 --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireConfirm(campaignDeactivateConfirm,
			"this permanently deactivates campaign "+args[0]+". It cannot be undone: deactivation ends "+
				"message delivery for the campaign and removes it from Bandwidth, and any phone numbers "+
				"assigned to it stop working for messaging. Pass --confirm to proceed."); err != nil {
			return err
		}

		svc, err := service(cmd)
		if err != nil {
			return err
		}
		if err := svc.DeactivateCampaign(args[0]); err != nil {
			return roleGateError(err, "Campaign Management")
		}

		// The delete returning is only an accept, not a completion: deactivation
		// is asynchronous, so "deactivated" starts false and stays false unless
		// and until a 404 on the follow-up read actually proves it happened —
		// not on the strength of the 204 alone, and not merely because --wait
		// gave up waiting. Same discipline as 'brand delete' — see its RunE.
		receipt := map[string]any{
			"id":          args[0],
			"deactivated": false,
			"status":      "accepted",
			"note": "deactivation accepted but not yet confirmed. Confirm with 'band tendlc campaign get " +
				args[0] + "' — a 404 means it is gone.",
		}

		if !campaignDeactivateWait {
			format, _ := cmdutil.OutputFlags(cmd)
			return output.Stdout(format, receipt)
		}

		// GoneIsDone: true — a 404 on the follow-up read means the deactivation
		// has actually taken effect. This is the only poll in the campaign
		// command set that treats 404 as success rather than "not ready yet".
		//
		// confirmedFetch wraps fetchCampaign so the receipt only ever claims
		// deactivated:true at the exact moment a 404 proves it. awaitTerminal
		// prints this same receipt map on BOTH the confirmed-success path and
		// the --timeout path, so if the deadline arrives before this ever
		// fires, deactivated is still false and the note above is still there
		// — the timeout receipt (exit 5) never contradicts its own exit code.
		fetch := fetchCampaign(svc, args[0])
		confirmedFetch := func() (map[string]any, bool, error) {
			obj, found, ferr := fetch()
			if ferr == nil && !found {
				receipt["deactivated"] = true
				delete(receipt, "note")
			}
			return obj, found, ferr
		}

		target := pollTarget{
			Noun:       "campaign",
			Fetch:      confirmedFetch,
			GoneIsDone: true,
		}
		return awaitTerminal(cmd, target, receipt, time.Duration(campaignDeactivateTimeout)*time.Second, campaignCreatePollInterval)
	},
}

var campaignNudgeCmd = &cobra.Command{
	Use:   "nudge <campaign-id>",
	Short: "Ask TCR to re-evaluate a campaign",
	Long: `Asks TCR to re-evaluate a campaign awaiting review.

Requires --intent, one of: ` + strings.Join(tendlcsvc.NudgeIntents, ", ") + `. APPEAL_REJECTION is
the documented path for a DECLINED campaign — it is exactly what a failed
'campaign create --wait' or 'campaign get' points a DECLINED campaign at.
REVIEW asks for another look at a campaign otherwise stuck pending.

--description is optional free-text context for the reviewer. The API caps
it at 1024 characters server-side; that limit is not duplicated here.

The endpoint returns 204 with no body, so this prints a synthesized receipt
confirming what was requested rather than a resource read back from the API.

A nudge asks for re-evaluation; it is neither destructive nor billable, so
unlike 'campaign deactivate' this does not require --confirm.`,
	Example: `  band tendlc campaign nudge CEXMPL1 --intent APPEAL_REJECTION --plain
  band tendlc campaign nudge CEXMPL1 --intent REVIEW --description "Updated sample messages per carrier feedback." --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if campaignNudgeIntent == "" {
			return cmdutil.NewMissingFlagsError([]string{"intent"})
		}
		if !validNudgeIntent(campaignNudgeIntent) {
			return cmdutil.NewFlagError("--intent " + campaignNudgeIntent + " is not valid; must be one of: " +
				strings.Join(tendlcsvc.NudgeIntents, ", "))
		}

		svc, err := service(cmd)
		if err != nil {
			return err
		}

		// The wire key is nudgeIntent, not intent — enumNudgeIntent's field
		// name and this command's --intent flag deliberately differ, and this
		// is the one spot that mismatch has to be gotten right. description is
		// included only when the flag was actually passed: an omitted flag and
		// an explicitly empty "" are not the same request.
		body := map[string]any{"nudgeIntent": campaignNudgeIntent}
		if cmd.Flags().Changed("description") {
			body["description"] = campaignNudgeDescription
		}

		if err := svc.NudgeCampaign(args[0], body); err != nil {
			return roleGateError(err, "Campaign Management")
		}

		receipt := map[string]any{
			"id":          args[0],
			"nudged":      true,
			"nudgeIntent": campaignNudgeIntent,
			"status":      "accepted",
			"check":       "band tendlc campaign get " + args[0],
		}
		if cmd.Flags().Changed("description") {
			receipt["description"] = campaignNudgeDescription
		}
		format, _ := cmdutil.OutputFlags(cmd)
		return output.Stdout(format, receipt)
	},
}

// validNudgeIntent reports whether intent is one of tendlcsvc.NudgeIntents.
func validNudgeIntent(intent string) bool {
	for _, v := range tendlcsvc.NudgeIntents {
		if v == intent {
			return true
		}
	}
	return false
}
