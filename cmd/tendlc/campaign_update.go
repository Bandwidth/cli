package tendlc

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	tendlcsvc "github.com/Bandwidth/cli/internal/tendlc"
)

var campaignUpdateOpts tendlcsvc.CampaignUpdateOptions

func init() {
	f := campaignUpdateCmd.Flags()
	f.StringVar(&campaignUpdateOpts.CampaignName, "campaign-name", "", "Campaign name")
	f.StringVar(&campaignUpdateOpts.Description, "description", "", "Campaign description")
	f.StringVar(&campaignUpdateOpts.Sample1, "sample1", "", "Sample message 1")
	f.StringVar(&campaignUpdateOpts.Sample2, "sample2", "", "Sample message 2")
	f.StringVar(&campaignUpdateOpts.Sample3, "sample3", "", "Sample message 3")
	f.StringVar(&campaignUpdateOpts.Sample4, "sample4", "", "Sample message 4")
	f.StringVar(&campaignUpdateOpts.Sample5, "sample5", "", "Sample message 5")
	f.StringVar(&campaignUpdateOpts.MessageFlow, "message-flow", "", "How a subscriber opts in and what they receive")
	f.StringVar(&campaignUpdateOpts.HelpMessage, "help-message", "", "Response sent for a HELP keyword")
	f.StringVar(&campaignUpdateOpts.HelpKeywords, "help-keywords", "", "HELP keywords, comma-separated")
	f.StringVar(&campaignUpdateOpts.OptinMessage, "optin-message", "", "Message sent confirming opt-in")
	f.StringVar(&campaignUpdateOpts.OptinKeywords, "optin-keywords", "", "Opt-in keywords, comma-separated")
	f.StringVar(&campaignUpdateOpts.OptoutMessage, "optout-message", "", "Message sent confirming opt-out")
	f.StringVar(&campaignUpdateOpts.OptoutKeywords, "optout-keywords", "", "Opt-out keywords, comma-separated")
	f.StringVar(&campaignUpdateOpts.PrivacyPolicyLink, "privacy-policy-link", "", "Link to the privacy policy")
	f.StringVar(&campaignUpdateOpts.TermsAndConditionsLink, "terms-and-conditions-link", "", "Link to the terms and conditions")
	f.StringVar(&campaignUpdateOpts.EmbeddedLinkSample, "embedded-link-sample", "", "Sample message containing an embedded link")
	f.BoolVar(&campaignUpdateOpts.EmbeddedLink, "embedded-link", false, "Messages contain embedded links")
	f.BoolVar(&campaignUpdateOpts.EmbeddedPhone, "embedded-phone", false, "Messages contain embedded phone numbers")
	f.BoolVar(&campaignUpdateOpts.NumberPool, "number-pool", false, "Campaign uses a shared number pool")
	f.BoolVar(&campaignUpdateOpts.AgeGated, "age-gated", false, "Campaign content requires age verification")
	f.BoolVar(&campaignUpdateOpts.DirectLending, "direct-lending", false, "Campaign involves direct lending or loan arrangement")
	f.BoolVar(&campaignUpdateOpts.AutoRenewal, "auto-renewal", false, "Campaign automatically renews")
	campaignCmd.AddCommand(campaignUpdateCmd)
}

var campaignUpdateCmd = &cobra.Command{
	Use:   "update <campaign-id>",
	Short: "Update a 10DLC campaign",
	Long: `Updates a 10DLC campaign.

For a direct campaign, the API replaces the whole record on update, so this
command reads the campaign first and sends it back with your changes
applied. Fields you do not pass are preserved; passing a flag with an empty
value clears that field (description, message-flow, and sample1 are
required on every campaign and cannot be cleared this way). For an imported
campaign, the update contract is narrower: only --campaign-name is accepted,
and passing any other flag is rejected outright rather than silently
discarded — see 'band tendlc campaign get' to check whether a campaign is
imported before deciding which flags apply.

This command prints an acceptance receipt, not the updated campaign: the PUT
here returns a bare {bandwidthId, campaignId}, and the change itself may take
several minutes to be reflected in 'campaign get' or 'campaign history' —
re-check after a few minutes to confirm it actually applied.

Editing a campaign is not cosmetic. Measured against production, an update
pushes the change to TCR and the campaign is then re-imported from TCR,
re-entering carrier review: the activity log records "Successfully updated
campaign", then "Campaign re-imported from TCR", then "Campaign is PENDING
with BANDWIDTH_DCA" — all within about four seconds. A REGISTERED campaign
that gets edited goes back in front of a DCA, and TCR is authoritative
afterward: any value TCR normalizes or rejects on re-import will silently
revert, with no error surfaced back through this command.

Also measured, and worth knowing before relying on a boolean flag: a boolean
set to true here applied, but two separate attempts to set that same field
back to false each returned 202 while leaving the stored value unchanged —
the second attempt did not even bump modifiedDate. Whether the API is
dropping false or TCR is re-asserting the prior value on re-import is not
established. Treat a boolean flip as unverified until you re-check the
campaign; do not assume a 202 means the value actually changed.

No --wait: this write lands against a campaign usually already in a
terminal state, so polling for one here would return immediately and report
success before the change (or the re-import above) actually took effect.

No --confirm: unlike 'brand update', a campaign update carries no fee and no
identity reset — the brand identity-verification rules do not transfer to
campaigns, so there is no API-justified reason to gate this behind a flag.`,
	Example: `  band tendlc campaign update CEXMPL1 --description "Updated description" --plain
  band tendlc campaign update CEXMPL1 --age-gated=false --plain
  band tendlc campaign update CEXMPL1 --campaign-name "Acme Notifications v2" --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		changed := map[string]bool{}
		any := false
		for _, name := range tendlcsvc.CampaignUpdateFieldFlags {
			if cmd.Flags().Changed(name) {
				changed[name] = true
				any = true
			}
		}
		if !any {
			return cmdutil.NewFlagError(
				"nothing to update — pass at least one of " + flagList(tendlcsvc.CampaignUpdateFieldFlags))
		}

		svc, err := service(cmd)
		if err != nil {
			return err
		}

		env, err := svc.GetCampaign(cmd.Context(), args[0])
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}
		current, err := env.Object()
		if err != nil {
			return err
		}

		body, err := tendlcsvc.BuildCampaignUpdateRequest(current, campaignUpdateOpts, changed)
		if err != nil {
			return err
		}

		updated, err := svc.UpdateCampaign(cmd.Context(), args[0], body)
		if err != nil {
			return campaignUpdateConflictHint(args[0], err)
		}

		// PUT /campaigns/{id} returns an ACCEPTANCE, not the updated resource:
		// measured against production, the body is a bare {bandwidthId,
		// campaignId}, so this reuses buildAcceptedCampaignReceipt (the same
		// shape 'campaign create'/'campaign sync' print) rather than printing
		// updated.Object() as though it were the campaign — see that helper's
		// doc comment for why output.Stdout, not output.StdoutAuto, is used.
		receipt, bandwidthID, err := buildAcceptedCampaignReceipt(cmd, updated)
		if err != nil {
			return err
		}
		receipt["note"] = "this is an acceptance, not the updated campaign: the change may take several " +
			"minutes to apply, and production re-imports the campaign from TCR after every update " +
			"(re-entering carrier review), so TCR is authoritative afterward and a value it normalizes " +
			"or rejects can silently revert. Check 'band tendlc campaign get " + bandwidthID +
			"' again shortly, or 'band tendlc campaign history " + bandwidthID + "' to watch the activity log."
		format, _ := cmdutil.OutputFlags(cmd)
		return output.Stdout(format, receipt)
	},
}

// campaignUpdateConflictHint maps a 409 on the update PUT to its specific
// cause: the campaign exists in Bandwidth but TCR has not caught up with it
// yet. Any other error still goes through roleGateError, so a 403 on update
// gets the same targeted message every other tendlc write does. Modeled on
// brandUpdateConflictHint in brand_update.go.
func campaignUpdateConflictHint(campaignID string, err error) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 409 {
		return &cmdutil.ConflictError{
			Message: "campaign " + campaignID + " exists in Bandwidth but not yet in TCR; run 'band tendlc campaign sync " +
				campaignID + "' and retry",
			Cause: err,
		}
	}
	return roleGateError(err, "Campaign Management")
}
