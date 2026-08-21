package tendlc

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	numberListLimit              int
	numberListOffset             int
	numberListAll                bool
	numberListCampaignIDContains string

	numberHistoryLimit  int
	numberHistoryOffset int
	numberHistoryAll    bool
)

func init() {
	lf := numberListCmd.Flags()
	lf.IntVar(&numberListLimit, "limit", 50, "Page size")
	lf.IntVar(&numberListOffset, "offset", 0, "Pagination offset")
	lf.BoolVar(&numberListAll, "all", false, "Fetch every page (cannot be combined with --offset)")
	lf.StringVar(&numberListCampaignIDContains, "campaign-id-contains", "",
		"Filter by campaign ID substring (e.g. CEXMPL1 also matches CEXMPL12); the API has no exact-match operator for this field")

	hf := numberHistoryCmd.Flags()
	hf.IntVar(&numberHistoryLimit, "limit", 50, "Page size")
	hf.IntVar(&numberHistoryOffset, "offset", 0, "Pagination offset")
	hf.BoolVar(&numberHistoryAll, "all", false, "Fetch every page (cannot be combined with --offset)")

	// numberGetCmd (Use: "number <phoneNumber>") is declared in numbers.go --
	// the legacy flat `band tendlc number <tn>` command, deleted in Task 3,
	// not here. It is already registered under Cmd by numbers.go's own
	// init(). A second, sibling command also named "number" would collide:
	// cobra's Command.Find matches children by Name() and returns the FIRST
	// one added to the parent's command slice, with no tie-break on Args or
	// anything else. Go initializes files within a package in filename
	// order, so this file's init() would run before numbers.go's, and
	// `Cmd.AddCommand(numberCmd)` here would always win the race -- silently
	// shadowing the legacy command's `Cmd.AddCommand(numberGetCmd)` call
	// below it, so a bare `band tendlc number +15555550100` would resolve to
	// this file's parent, find no subcommand named "+15555550100", and print
	// this command's help instead of running the legacy lookup. Verified
	// against a cobra sandbox before writing this.
	//
	// Attaching these three subcommands directly onto the existing
	// numberGetCmd node avoids that entirely: no second "number" command is
	// ever registered. Cobra falls through to a parent's own Args/RunE
	// whenever the next token isn't a known child's name, so
	// `band tendlc number <tn>` keeps invoking the legacy runNumberGet
	// unchanged, while `list`, `get <tn>`, and `history <tn>` route to the
	// commands below. When Task 3 deletes numbers.go, replace numberGetCmd
	// here with a plain `numberCmd` parent (Use: "number") and re-register
	// it on Cmd directly.
	numberGetCmd.AddCommand(numberListCmd, numberDetailCmd, numberHistoryCmd)
}

var numberListCmd = &cobra.Command{
	Use:   "list",
	Short: "List 10DLC registered phone numbers",
	Long: `Lists the phone numbers registered for 10DLC traffic on the account.

There is no --status flag. That is deliberate and measured, not an
oversight: status[eq] and status[contains] are both accepted and silently
ignored, for every value tried -- including a value matching nothing at
all -- and every one of them returned every phone number on the account
regardless. A filter that returns every record with a 200 and no error is
worse than an absent flag, because the caller believes it worked -- the
same reasoning that already removed 'brand list --bandwidth-id' and
'campaign list --usecase'.

--campaign-id-contains, by contrast, genuinely narrows results:
campaignId[contains] filters correctly. campaignId[eq] does not -- like
status, it is accepted and silently ignored, returning every number
regardless of value -- which is why the flag is named for what it actually
does (a substring match) rather than implying an exact-match filter the API
cannot perform. For the campaign-scoped view, which is a different endpoint,
use 'band tendlc campaign numbers <campaign-id>' instead.

The list projection has two shapes, not one fixed set of keys: every record
carries createdDate, modifiedDate, nnid, phoneNumber, and status; a number
already assigned to a campaign additionally carries brandId, campaignId,
and customerProfileId. Do not assume every record is missing the campaign
fields -- check for them rather than relying on their absence.`,
	Example: `  band tendlc number list --plain
  band tendlc number list --all --plain
  band tendlc number list --campaign-id-contains CEXMPL1 --plain`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Detected via Changed so that an explicit --offset 0 also conflicts.
		if numberListAll && cmd.Flags().Changed("offset") {
			return cmdutil.NewFlagError("--all fetches every page, so it cannot be combined with --offset")
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)

		var filters []api.Filter
		if numberListCampaignIDContains != "" {
			filters = append(filters, api.Filter{Field: "campaignId", Op: api.OpContains, Value: numberListCampaignIDContains})
		}

		if !numberListAll {
			env, err := svc.ListPhoneNumbers(numberListLimit, numberListOffset, filters)
			if err != nil {
				return roleGateError(err, "Campaign Management")
			}
			items, err := env.List()
			if err != nil {
				return err
			}
			warnIfTruncated(cmd, env, numberListOffset, len(items), "phone numbers")
			return output.StdoutPlainList(format, plain, items)
		}

		var all []any
		err = api.ForEachPage(func(limit, offset int) (*api.Envelope, error) {
			return svc.ListPhoneNumbers(limit, offset, filters)
		}, numberListLimit, func(batch []any) error {
			all = append(all, batch...)
			return nil
		})
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}
		if all == nil {
			all = []any{}
		}
		return output.StdoutPlainList(format, plain, all)
	},
}

// numberDetailCmd implements `band tendlc number get <tn>`. Named
// numberDetailCmd, not numberGetCmd, because numbers.go already declares
// numberGetCmd for the legacy flat `number <tn>` command this is attached
// beneath -- see this file's init().
var numberDetailCmd = &cobra.Command{
	Use:   "get <phone-number>",
	Short: "Get 10DLC registration details for a phone number",
	Long: `Shows one phone number's 10DLC registration record.

Shipped plainly, with no special-case error handling: a 404 maps to exit 3
through the normal path. On the one account this was tested against, this
endpoint returned 404 for every number tried, while 'number history' on the
same path prefix returned 200 for all of them. The cause is unconfirmed and
may be account-specific -- this API reports authorization failures as 403,
so a 404 here is not a permissions mask in disguise, but one account isn't
enough to call it an API defect either. The command starts working
wherever the endpoint does, without a bespoke error message guessing at why.`,
	Example: `  band tendlc number get +15555550100 --plain`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		env, err := svc.GetPhoneNumber(args[0])
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}
		obj, err := env.Object()
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)
		return output.StdoutAuto(format, plain, obj)
	},
}

var numberHistoryCmd = &cobra.Command{
	Use:   "history <phone-number>",
	Short: "Show a phone number's activity log",
	Long: `Lists a phone number's activity log: free-text {createdDate, message}
entries, newest first.

As with brand and campaign history, there are no versioned snapshots and no
per-entry fetch -- this is the only history view for a phone number.`,
	Example: `  band tendlc number history +15555550100 --plain
  band tendlc number history +15555550100 --all --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if numberHistoryAll && cmd.Flags().Changed("offset") {
			return cmdutil.NewFlagError("--all fetches every page, so it cannot be combined with --offset")
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)

		if !numberHistoryAll {
			env, err := svc.PhoneNumberHistory(args[0], numberHistoryLimit, numberHistoryOffset)
			if err != nil {
				return roleGateError(err, "Campaign Management")
			}
			items, err := env.List()
			if err != nil {
				return err
			}
			warnIfTruncated(cmd, env, numberHistoryOffset, len(items), "history entries")
			return output.StdoutPlainList(format, plain, items)
		}

		var all []any
		err = api.ForEachPage(func(limit, offset int) (*api.Envelope, error) {
			return svc.PhoneNumberHistory(args[0], limit, offset)
		}, numberHistoryLimit, func(batch []any) error {
			all = append(all, batch...)
			return nil
		})
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}
		if all == nil {
			all = []any{}
		}
		return output.StdoutPlainList(format, plain, all)
	},
}
