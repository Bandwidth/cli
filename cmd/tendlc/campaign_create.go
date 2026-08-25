package tendlc

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	tendlcsvc "github.com/Bandwidth/cli/internal/tendlc"
)

var (
	campaignCreateOpts    tendlcsvc.CampaignCreateOptions
	campaignCreateWait    bool
	campaignCreateTimeout int

	campaignSyncName string
)

// campaignCreatePollInterval is how often --wait re-checks the campaign
// while it registers. Campaign registration runs through TCR, an external
// registry, so a slower interval than the 2-second polls used for
// Bandwidth-internal operations elsewhere in this CLI is appropriate here —
// the same reasoning as brandCreatePollInterval.
const campaignCreatePollInterval = 5 * time.Second

// campaignCreateBoolFlags are the ten boolean flags on `campaign create`, in
// the same order internal/tendlc's campaignCreateBoolFields lists them.
// BuildCampaignCreateRequest reads intent from a changed map keyed on these
// exact names, not from the option struct's zero values, since false is a
// real value here (see BuildCampaignCreateRequest's doc comment) — this list
// is what lets RunE build that map from cmd.Flags().Changed.
var campaignCreateBoolFlags = []string{
	"embedded-link", "embedded-phone", "terms-and-conditions", "number-pool",
	"age-gated", "direct-lending", "subscriber-optin", "subscriber-optout",
	"subscriber-help", "auto-renewal",
}

func init() {
	f := campaignCreateCmd.Flags()
	f.StringVar(&campaignCreateOpts.BrandID, "brand-id", "", "Brand backing this campaign (required)")
	f.StringVar(&campaignCreateOpts.CampaignName, "campaign-name", "", "Campaign name")
	f.StringVar(&campaignCreateOpts.Usecase, "usecase", "", "Campaign usecase (required)")
	f.StringSliceVar(&campaignCreateOpts.SubUsecases, "sub-usecase", nil, "Campaign sub-usecase(s), comma-separated or repeated")
	f.StringVar(&campaignCreateOpts.Description, "description", "", "Campaign description (required)")
	f.StringVar(&campaignCreateOpts.Sample1, "sample1", "", "Sample message 1 (required)")
	f.StringVar(&campaignCreateOpts.Sample2, "sample2", "", "Sample message 2")
	f.StringVar(&campaignCreateOpts.Sample3, "sample3", "", "Sample message 3")
	f.StringVar(&campaignCreateOpts.Sample4, "sample4", "", "Sample message 4")
	f.StringVar(&campaignCreateOpts.Sample5, "sample5", "", "Sample message 5")
	f.StringVar(&campaignCreateOpts.MessageFlow, "message-flow", "", "How a subscriber opts in and what they receive (required)")
	f.StringVar(&campaignCreateOpts.HelpMessage, "help-message", "", "Response sent for a HELP keyword")
	f.StringVar(&campaignCreateOpts.HelpKeywords, "help-keywords", "", "HELP keywords, comma-separated")
	f.StringVar(&campaignCreateOpts.OptinMessage, "optin-message", "", "Message sent confirming opt-in")
	f.StringVar(&campaignCreateOpts.OptinKeywords, "optin-keywords", "", "Opt-in keywords, comma-separated")
	f.StringVar(&campaignCreateOpts.OptoutMessage, "optout-message", "", "Message sent confirming opt-out")
	f.StringVar(&campaignCreateOpts.OptoutKeywords, "optout-keywords", "", "Opt-out keywords, comma-separated")
	f.StringVar(&campaignCreateOpts.PrivacyPolicyLink, "privacy-policy-link", "", "Link to the privacy policy")
	f.StringVar(&campaignCreateOpts.TermsAndConditionsLink, "terms-and-conditions-link", "", "Link to the terms and conditions")
	f.StringVar(&campaignCreateOpts.EmbeddedLinkSample, "embedded-link-sample", "", "Sample message containing an embedded link")
	f.BoolVar(&campaignCreateOpts.EmbeddedLink, "embedded-link", false, "Messages contain embedded links")
	f.BoolVar(&campaignCreateOpts.EmbeddedPhone, "embedded-phone", false, "Messages contain embedded phone numbers")
	f.BoolVar(&campaignCreateOpts.TermsAndConditions, "terms-and-conditions", false, "Subscribers must accept terms and conditions to opt in")
	f.BoolVar(&campaignCreateOpts.NumberPool, "number-pool", false, "Campaign uses a shared number pool")
	f.BoolVar(&campaignCreateOpts.AgeGated, "age-gated", false, "Campaign content requires age verification")
	f.BoolVar(&campaignCreateOpts.DirectLending, "direct-lending", false, "Campaign involves direct lending or loan arrangement")
	f.BoolVar(&campaignCreateOpts.SubscriberOptin, "subscriber-optin", false, "Subscribers explicitly opt in")
	f.BoolVar(&campaignCreateOpts.SubscriberOptout, "subscriber-optout", false, "Subscribers can opt out")
	f.BoolVar(&campaignCreateOpts.SubscriberHelp, "subscriber-help", false, "Subscribers can request help")
	f.BoolVar(&campaignCreateOpts.AutoRenewal, "auto-renewal", false, "Campaign automatically renews")
	f.BoolVar(&campaignCreateWait, "wait", false, "Block until the campaign's status reaches a terminal state")
	f.IntVar(&campaignCreateTimeout, "timeout", 300, "Seconds to wait when --wait is set")
	campaignCmd.AddCommand(campaignCreateCmd)

	campaignSyncCmd.Flags().StringVar(&campaignSyncName, "campaign-name", "", "Campaign name to send along with the sync")
	campaignCmd.AddCommand(campaignSyncCmd)
}

var campaignCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Register a 10DLC campaign",
	Long: `Registers a 10DLC campaign against an existing, verified brand.

Before submitting, this command reads the brand named by --brand-id.
Whether POST /campaigns silently discards an invalid brandId the way POST
/brands was measured to silently discard an invalid customerProfileId (see
'band tendlc brand create') is unmeasured, so this guards the same way: a
404 on that read stops the create here, naming the bad ID. A 403, or any
other pre-flight failure, does not stop it — it proceeds with a one-line
warning on stderr instead, the same degrade-don't-block behavior as the
brand create pre-flight.

If the brand read succeeds, its identity status is checked too: a campaign
requires a brand at VERIFIED, VETTED_VERIFIED, or SELF_DECLARED, so any
other status stops the create here naming the blocking state, rather than
letting the API return an opaque error later.

This is a non-idempotent, billable write. After an ambiguous failure — a
timeout or a dropped connection — do not blindly retry: list campaigns
filtered by --brand-id-contains and reconcile against what you submitted
first. Retrying blind risks a second campaign against the same brand.`,
	Example: `  band tendlc campaign create --brand-id BEXMPL1 --usecase ACCOUNT_NOTIFICATION \
    --description "Sends account notifications to opted-in subscribers." \
    --sample1 "Your account balance is now available. Reply STOP to opt out." \
    --message-flow "Customer opts in via web form; campaign sends account notifications only." \
    --help-message "For help, reply HELP." --help-keywords HELP,INFO

  band tendlc campaign create --brand-id BEXMPL1 --usecase 2FA \
    ... --wait --timeout 600`,
	// No positional args: this is a non-idempotent, billable create, so a
	// stray positional must be rejected rather than silently ignored and
	// creating an unintended campaign — see
	// TestBrandCommandsRejectStrayPositionals's comment on the PR 2 incident
	// this guards against.
	Args: cobra.NoArgs,
	// Required-ness is enforced in RunE, not via MarkFlagRequired: cobra
	// rejects before RunE, which reports one flag at a time and would block a
	// future interactive prompt from filling them in.
	RunE: func(cmd *cobra.Command, args []string) error {
		// Built before validation, not just before the build request below:
		// ValidateCampaignCreate needs to see which of the three
		// subscriber-optin/optout/help booleans were explicitly passed, since
		// only their presence — not their value — is tier 2 of the create
		// requirement tree (see the function's doc comment).
		changed := map[string]bool{}
		for _, name := range campaignCreateBoolFlags {
			if cmd.Flags().Changed(name) {
				changed[name] = true
			}
		}

		advisory, err := tendlcsvc.ValidateCampaignCreate(campaignCreateOpts, changed)
		if err != nil {
			return err
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		if advisory != "" {
			cmd.PrintErrln(advisory)
		}

		if err := preflightBrand(cmd, svc, campaignCreateOpts.BrandID); err != nil {
			return err
		}

		env, err := svc.CreateCampaign(cmd.Context(), tendlcsvc.BuildCampaignCreateRequest(campaignCreateOpts, changed))
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}

		receipt, bandwidthID, err := buildAcceptedCampaignReceipt(cmd, env)
		if err != nil {
			return err
		}

		if !campaignCreateWait {
			format, _ := cmdutil.OutputFlags(cmd)
			return output.Stdout(format, receipt)
		}

		target := pollTarget{
			Noun:  "campaign",
			Fetch: fetchCampaign(cmd.Context(), svc, bandwidthID),
			Classify: func(o map[string]any) tendlcsvc.StateClass {
				status, _ := o["status"].(string)
				return tendlcsvc.ClassifyCampaignStatus(status)
			},
			Remediate: func(o map[string]any) string {
				status, _ := o["status"].(string)
				// CampaignRemediation embeds a literal "<campaign-id>" placeholder
				// in its DECLINED/ERROR text because the classifier only ever sees
				// a status string, not the ID. The ID is known here, so it is
				// substituted at this call site rather than changing the
				// classifier's signature.
				return strings.ReplaceAll(tendlcsvc.CampaignRemediation(status), "<campaign-id>", bandwidthID)
			},
			LastSeenStatus: func(o map[string]any) string {
				status, _ := o["status"].(string)
				return status
			},
		}
		return awaitTerminal(cmd, target, receipt, time.Duration(campaignCreateTimeout)*time.Second, campaignCreatePollInterval)
	},
}

var campaignSyncCmd = &cobra.Command{
	Use:   "sync <campaign-id>",
	Short: "Re-pull a campaign's current state from TCR",
	Long: `Syncs a campaign by re-pulling its current state from TCR — for import and
direct customers alike.

This is not a create: it posts to the same POST /campaigns endpoint create
uses, but a body containing only campaignId (plus campaignName when
supplied) is what makes it a sync instead of a new registration. Sending any
other key would turn it back into a create, so nothing else is added.

No --wait: this writes against a campaign that is usually already in a
terminal state, so polling for one here would return immediately and report
success before the sync actually applied.`,
	Example: `  band tendlc campaign sync CEXMPL1 --plain
  band tendlc campaign sync CEXMPL1 --campaign-name "Acme Notifications" --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		env, err := svc.CreateCampaign(cmd.Context(), tendlcsvc.BuildCampaignSyncRequest(args[0], campaignSyncName))
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}
		receipt, _, err := buildAcceptedCampaignReceipt(cmd, env)
		if err != nil {
			return err
		}
		format, _ := cmdutil.OutputFlags(cmd)
		return output.Stdout(format, receipt)
	},
}

// preflightBrand reads the brand named by brandID before create writes the
// campaign — see campaignCreateCmd's Long for why. It degrades rather than
// blocks on transport/permission failures: only a definitive 404 (the brand
// does not exist) stops the create. A 403, a failure to build the service,
// or any other error is reported on stderr and swallowed, matching
// preflightCustomerProfile's degrade-don't-block behavior in
// brand_create.go.
//
// Unlike preflightCustomerProfile, a successful read is not automatically a
// pass: a campaign requires a brand whose identity status is a success state,
// so any other identity status is a blocking condition and returns a
// ConflictError (exit 4) naming the status, rather than letting the create
// proceed toward an opaque API failure.
//
// The allow-list is derived from ClassifyBrandIdentity's StateSuccess set
// (VERIFIED, VETTED_VERIFIED, SELF_DECLARED) rather than a second hardcoded
// list, so the two cannot drift apart the way this one already had from
// AGENTS.md's documented `brand create --wait` success set. SELF_DECLARED in
// particular is the sole-proprietor brand path, and 'band tendlc brand
// create --wait' already exits 0 on it with the full brand object — nothing
// has ever measured whether POST /campaigns itself accepts a SELF_DECLARED
// brandId, since a client-side rejection here means the request is never
// even attempted. Blocking it anyway would be an unmeasured guess that locks
// a real 10DLC customer class out of the CLI entirely; letting it through
// costs at most one opaque API error if the API turns out to disagree. That
// is the safer direction to be wrong in, so this lets the API speak instead
// of guessing.
func preflightBrand(cmd *cobra.Command, svc *tendlcsvc.Service, brandID string) error {
	env, err := svc.GetBrand(cmd.Context(), brandID)
	if err == nil {
		var obj map[string]any
		obj, err = env.Object()
		if err == nil {
			status, _ := obj["brandIdentityStatus"].(string)
			if tendlcsvc.ClassifyBrandIdentity(status) == tendlcsvc.StateSuccess {
				return nil
			}
			return &cmdutil.ConflictError{Message: fmt.Sprintf(
				"brand %q has identity status %q; a campaign requires a verified brand (VERIFIED, "+
					"VETTED_VERIFIED, or SELF_DECLARED) — run 'band tendlc brand get %s' to check its current status",
				brandID, status, brandID)}
		}
	}
	if isNotFound(err) {
		return fmt.Errorf("brand %q not found — run 'band tendlc brand list' to see valid brand IDs (%w)", brandID, err)
	}
	cmd.PrintErrf("warning: could not verify brand %q before creating the campaign (%v); proceeding anyway\n", brandID, err)
	return nil
}

// buildAcceptedCampaignReceipt turns a campaign-write acceptance response —
// the POST /campaigns 202 that both create and sync get — into the receipt
// shape both print: {bandwidthId, campaignId (if present), status, resume}.
// bandwidthId exists immediately; campaignId is assigned by TCR and is
// commonly absent until registration completes, so it is omitted entirely
// rather than sent as null. Mirrors buildAcceptedReceipt in brand_create.go.
//
// If the response carries no bandwidthId, there is no ID to poll or resume
// with, so the caller must not proceed. This prints whatever the body
// actually was via output.Stdout, not StdoutAuto: env.Data is real API data,
// not a synthetic receipt, but it can still be a single-key map, and
// FlattenResponse unwraps any single-key map — under --plain that would drop
// the key and print a bare value instead of the object it came from. See
// async.go's emitReceipt for the same reasoning applied to synthetic
// receipts.
func buildAcceptedCampaignReceipt(cmd *cobra.Command, env *api.Envelope) (receipt map[string]any, bandwidthID string, err error) {
	obj, objErr := env.Object()
	bandwidthID, _ = obj["bandwidthId"].(string)
	if objErr != nil || bandwidthID == "" {
		format, _ := cmdutil.OutputFlags(cmd)
		if writeErr := output.Stdout(format, env.Data); writeErr != nil {
			cmd.PrintErrln(fmt.Sprintf("writing response: %v", writeErr))
		}
		return nil, "", fmt.Errorf("campaign response did not include a bandwidthId")
	}

	receipt = map[string]any{
		"bandwidthId": bandwidthID,
		"status":      "accepted",
		"resume":      "band tendlc campaign get " + bandwidthID,
	}
	if campaignID, ok := obj["campaignId"].(string); ok && campaignID != "" {
		receipt["campaignId"] = campaignID
	}
	return receipt, bandwidthID, nil
}
