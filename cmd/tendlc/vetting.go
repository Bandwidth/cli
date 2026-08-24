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

// vettingCmd is the `band tendlc vetting` parent.
//
// Every subcommand below takes a BRAND ID, not a vetting ID — vettings are
// brand-scoped, and there is no campaign vetting endpoint in either spec. A
// campaign's vettingStatus is a read-only field derived from its brand's
// vetting state; re-evaluating a campaign directly is a different command
// ('nudge').
var vettingCmd = &cobra.Command{
	Use:   "vetting",
	Short: "Order and record external vettings on a 10DLC brand",
	Long: `Order and record third-party vettings for a brand.

Vettings are brand-scoped: there is no campaign vetting endpoint in either
spec. A campaign only exposes a read-only, derived vettingStatus; requesting
a new evaluation for a campaign is a different command ('nudge').

Every command here accepts a brand ID as its first positional, not a
vetting ID.

Requires the Registration Center feature and the Campaign Management role.`,
	Args: cobra.NoArgs,
	// A trivial RunE is required, not decorative: cobra's execute() checks
	// Runnable() (Run/RunE set) BEFORE it ever calls ValidateArgs, so a
	// parent with no RunE always short-circuits to flag.ErrHelp regardless
	// of Args -- Args: cobra.NoArgs above would silently never run. Calling
	// cmd.Help() here keeps the existing bare-invocation behavior (print
	// help, exit 0) while letting a stray positional -- e.g. a deleted
	// command's name -- reach NoArgs and fail with a non-zero exit instead
	// of being silently swallowed as if it were a help request.
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(vettingCmd)
}

// vettingEvpProviders are the external vetting providers the API accepts. A
// small, stable, documented enum — unlike vettingClasses below, nothing here
// is undocumented.
var vettingEvpProviders = []string{"AEGIS", "CV", "WMC"}

// vettingClasses are the vetting classes the API accepts.
//
// RCS is included deliberately even though it is absent from the published
// enumVettingClass: production accepts it, confirmed by pairing each class
// with an invalid evpId and observing which produced a class-level error
// rather than an evp-level one. Dropping it to match the spec would reject a
// value production actually honors — do not "fix" this to match the spec.
var vettingClasses = []string{"STANDARD", "ENHANCED", "POLITICAL", "AUTHPLUS", "RCS"}

// vettingIDKeys are the two field names production uses for a vetting's ID.
// The vettings list returns it under bandwidthId; the spec shows
// vettingBandwidthId on the POST .../vettings 202. Both are checked
// wherever a vetting object is read, and whichever key is actually present is
// preserved as-is in receipts rather than normalized to one name — see
// buildVettingReceipt.
var vettingIDKeys = []string{"bandwidthId", "vettingBandwidthId"}

func isValidEnum(v string, enum []string) bool {
	for _, e := range enum {
		if e == v {
			return true
		}
	}
	return false
}

// validateVettingRequest reports every violation of --evp/--class in one
// error, the way the API reports every violation in one 400: a missing flag
// and an invalid value on the OTHER flag are combined rather than only the
// first being surfaced.
func validateVettingRequest(evp, class string) error {
	var missing []string
	var invalid []string

	switch {
	case evp == "":
		missing = append(missing, "evp")
	case !isValidEnum(evp, vettingEvpProviders):
		invalid = append(invalid, "--evp must be one of: "+strings.Join(vettingEvpProviders, ", "))
	}
	switch {
	case class == "":
		missing = append(missing, "class")
	case !isValidEnum(class, vettingClasses):
		invalid = append(invalid, "--class must be one of: "+strings.Join(vettingClasses, ", "))
	}

	if len(missing) == 0 && len(invalid) == 0 {
		return nil
	}
	if len(invalid) == 0 {
		return cmdutil.NewMissingFlagsError(missing)
	}
	msg := strings.Join(invalid, "; ")
	if len(missing) > 0 {
		prefixed := make([]string, len(missing))
		for i, m := range missing {
			prefixed[i] = "--" + m
		}
		msg += "; missing required flags: " + strings.Join(prefixed, ", ")
	}
	return cmdutil.NewFlagError(msg)
}

// buildVettingReceipt turns a vetting write's response (a POST .../vettings
// 202, or a PUT .../vettings/{id} response) into the receipt this command set
// prints: {<idField>: id, brandId, status, check}.
//
// idField (found internally, not returned — see below) is whichever of
// vettingIDKeys was actually present on the response: bandwidthId or
// vettingBandwidthId. It is preserved under its own name rather than
// normalized: normalizing would be tidier and would be wrong, since it would
// misreport which field the API actually sent.
//
// If the response carries neither key, there is no ID to poll or look up
// against the vettings list, so the caller must not proceed. This prints
// whatever the body actually was via output.Stdout, not StdoutAuto: env.Data
// is real API data, not a synthetic receipt, but it can still be a
// single-key map, and FlattenResponse unwraps any single-key map — under
// --plain that would drop the key and print a bare value instead of the
// object it came from. See async.go's emitReceipt for the same reasoning
// applied to synthetic receipts.
func buildVettingReceipt(cmd *cobra.Command, env *api.Envelope, brandID string) (receipt map[string]any, vettingID string, err error) {
	obj, objErr := env.Object()
	var idField string
	for _, key := range vettingIDKeys {
		if v, ok := obj[key].(string); ok && v != "" {
			idField, vettingID = key, v
			break
		}
	}
	if objErr != nil || vettingID == "" {
		format, _ := cmdutil.OutputFlags(cmd)
		if writeErr := output.Stdout(format, env.Data); writeErr != nil {
			cmd.PrintErrln(fmt.Sprintf("writing response: %v", writeErr))
		}
		return nil, "", fmt.Errorf("vetting response did not include a vetting ID")
	}

	receipt = map[string]any{
		idField:   vettingID,
		"brandId": brandID,
		"status":  "accepted",
		"check":   "band tendlc vetting list " + brandID,
	}
	return receipt, vettingID, nil
}

// fetchVetting adapts a ListVettings call into pollTarget.Fetch. There is no
// GET .../vettings/{id} — the vettings list is the only read surface for a
// vetting's status — so --wait re-lists (walking every page) and returns the
// entry whose ID, under either vettingIDKeys name, matches vettingID.
func fetchVetting(svc *tendlcsvc.Service, brandID, vettingID string) func() (map[string]any, bool, error) {
	return func() (map[string]any, bool, error) {
		const pageSize = 100
		offset := 0
		for {
			env, err := svc.ListVettings(brandID, pageSize, offset)
			if err != nil {
				if isNotFound(err) {
					return nil, false, nil
				}
				return nil, false, err
			}
			items, err := env.List()
			if err != nil {
				return nil, false, err
			}
			for _, item := range items {
				obj, ok := item.(map[string]any)
				if !ok {
					continue
				}
				for _, key := range vettingIDKeys {
					if v, _ := obj[key].(string); v == vettingID {
						return obj, true, nil
					}
				}
			}
			offset += len(items)
			if len(items) == 0 || env.Page == nil || !env.Page.Truncated(offset) {
				return nil, false, nil
			}
		}
	}
}

// classifyVettingObj adapts ClassifyVetting to pollTarget.Classify.
func classifyVettingObj(obj map[string]any) tendlcsvc.StateClass {
	status, _ := obj["vettingStatus"].(string)
	return tendlcsvc.ClassifyVetting(status)
}

// vettingPollInterval mirrors brandCreatePollInterval: vetting decisions run
// through an external provider, not a Bandwidth-internal process, so a
// slower interval than the 2-second polls used elsewhere in the CLI is
// appropriate here too.
const vettingPollInterval = 5 * time.Second

var (
	vettingListLimit  int
	vettingListOffset int
	vettingListAll    bool
)

func init() {
	f := vettingListCmd.Flags()
	f.IntVar(&vettingListLimit, "limit", 50, "Page size")
	f.IntVar(&vettingListOffset, "offset", 0, "Pagination offset")
	f.BoolVar(&vettingListAll, "all", false, "Fetch every page (cannot be combined with --offset)")
	vettingCmd.AddCommand(vettingListCmd)
}

var vettingListCmd = &cobra.Command{
	Use:   "list <brand-id>",
	Short: "List the external vettings recorded against a brand",
	Long: `Lists the external vettings recorded against a brand.

The positional here is a BRAND ID. Vettings are brand-scoped; there is no
campaign vetting endpoint.`,
	Example: `  band tendlc vetting list BGJR2BA --plain
  band tendlc vetting list BGJR2BA --all --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if vettingListAll && cmd.Flags().Changed("offset") {
			return cmdutil.NewFlagError("--all fetches every page, so it cannot be combined with --offset")
		}
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)

		if !vettingListAll {
			env, err := svc.ListVettings(args[0], vettingListLimit, vettingListOffset)
			if err != nil {
				return roleGateError(err, "Campaign Management")
			}
			items, err := env.List()
			if err != nil {
				return err
			}
			warnIfTruncated(cmd, env, vettingListOffset, len(items), "vettings")
			return output.StdoutPlainList(format, plain, items)
		}

		var all []any
		err = api.ForEachPage(func(limit, offset int) (*api.Envelope, error) {
			return svc.ListVettings(args[0], limit, offset)
		}, vettingListLimit, func(batch []any) error {
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

var (
	vettingRequestEvp     string
	vettingRequestClass   string
	vettingRequestConfirm bool
	vettingRequestWait    bool
	vettingRequestTimeout int
)

func init() {
	f := vettingRequestCmd.Flags()
	f.StringVar(&vettingRequestEvp, "evp", "", "External vetting provider: AEGIS, CV, WMC (required)")
	f.StringVar(&vettingRequestClass, "class", "", "Vetting class: STANDARD, ENHANCED, POLITICAL, AUTHPLUS, RCS (required)")
	f.BoolVar(&vettingRequestConfirm, "confirm", false, "Required. Confirms the billable order with the external vetting provider.")
	f.BoolVar(&vettingRequestWait, "wait", false, "Block until the vetting reaches a terminal state")
	f.IntVar(&vettingRequestTimeout, "timeout", 300, "Seconds to wait when --wait is set")
	vettingCmd.AddCommand(vettingRequestCmd)
}

var vettingRequestCmd = &cobra.Command{
	Use:   "request <brand-id>",
	Short: "Order a new external vetting for a brand",
	Long: `Orders a new third-party vetting for a BRAND — the positional here is a
brand ID, not a vetting ID; vettings are brand-scoped and there is no
campaign vetting endpoint.

--class accepts STANDARD, ENHANCED, POLITICAL, AUTHPLUS, and RCS. RCS is not
in the published enumVettingClass, but production accepts it — confirmed by
pairing each class with an invalid evpId and observing which produced a
class-level error.

This places a billable order with an external vetting provider, so
--confirm is required.`,
	Example: `  band tendlc vetting request BGJR2BA --evp AEGIS --class STANDARD --confirm --plain
  band tendlc vetting request BGJR2BA --evp AEGIS --class STANDARD --confirm --wait --plain`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateVettingRequest(vettingRequestEvp, vettingRequestClass); err != nil {
			return err
		}
		brandID := args[0]
		if err := requireConfirm(vettingRequestConfirm,
			fmt.Sprintf("requesting a %s vetting from %s for brand %s is a billable order placed with "+
				"an external vetting provider. Pass --confirm to proceed.",
				vettingRequestClass, vettingRequestEvp, brandID)); err != nil {
			return err
		}

		svc, err := service(cmd)
		if err != nil {
			return err
		}

		body := map[string]any{
			"evpId":        vettingRequestEvp,
			"vettingClass": vettingRequestClass,
		}
		env, err := svc.RequestVetting(brandID, body)
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}

		receipt, vettingID, err := buildVettingReceipt(cmd, env, brandID)
		if err != nil {
			return err
		}

		if !vettingRequestWait {
			format, _ := cmdutil.OutputFlags(cmd)
			return output.Stdout(format, receipt)
		}

		target := pollTarget{
			Noun:     "vetting",
			Fetch:    fetchVetting(svc, brandID, vettingID),
			Classify: classifyVettingObj,
		}
		return awaitTerminal(cmd, target, receipt, time.Duration(vettingRequestTimeout)*time.Second, vettingPollInterval)
	},
}

var (
	vettingImportEvp          string
	vettingImportVettingToken string
	vettingImportWait         bool
	vettingImportTimeout      int
)

func init() {
	f := vettingImportCmd.Flags()
	f.StringVar(&vettingImportEvp, "evp", "", "External vetting provider that performed the vetting: AEGIS, CV, WMC (required)")
	f.StringVar(&vettingImportVettingToken, "vetting-token", "", "Token issued by the vetting provider, if one exists")
	f.BoolVar(&vettingImportWait, "wait", false, "Block until the vetting reaches a terminal state")
	f.IntVar(&vettingImportTimeout, "timeout", 300, "Seconds to wait when --wait is set")
	vettingCmd.AddCommand(vettingImportCmd)
}

var vettingImportCmd = &cobra.Command{
	Use:   "import <brand-id> <vetting-id>",
	Short: "Record an already-performed external vetting against a brand",
	Long: `Records a vetting that was already performed outside Bandwidth against a
BRAND. The first positional is the brand ID; the second is the vetting ID
assigned by the external provider.

Recording an already-performed vetting is not billable, so unlike 'vetting
request' this takes no --confirm.`,
	Example: `  band tendlc vetting import BGJR2BA V123 --evp AEGIS --plain
  band tendlc vetting import BGJR2BA V123 --evp AEGIS --vetting-token TOK123 --plain`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if vettingImportEvp == "" {
			return cmdutil.NewMissingFlagsError([]string{"evp"})
		}
		if !isValidEnum(vettingImportEvp, vettingEvpProviders) {
			return cmdutil.NewFlagError("--evp must be one of: " + strings.Join(vettingEvpProviders, ", "))
		}

		brandID, vettingID := args[0], args[1]

		svc, err := service(cmd)
		if err != nil {
			return err
		}

		body := map[string]any{"evpId": vettingImportEvp}
		if cmd.Flags().Changed("vetting-token") {
			body["vettingToken"] = vettingImportVettingToken
		}

		env, err := svc.ImportVetting(brandID, vettingID, body)
		if err != nil {
			return roleGateError(err, "Campaign Management")
		}

		receipt, respVettingID, err := buildVettingReceipt(cmd, env, brandID)
		if err != nil {
			return err
		}
		// The response's own ID may differ in name from what the caller passed
		// (bandwidthId vs vettingBandwidthId), so the response's value — not the
		// positional — is what --wait polls for. respVettingID is always
		// non-empty here: buildVettingReceipt errors (and this function has
		// already returned) whenever it would otherwise come back empty.

		if !vettingImportWait {
			format, _ := cmdutil.OutputFlags(cmd)
			return output.Stdout(format, receipt)
		}

		target := pollTarget{
			Noun:     "vetting",
			Fetch:    fetchVetting(svc, brandID, respVettingID),
			Classify: classifyVettingObj,
		}
		return awaitTerminal(cmd, target, receipt, time.Duration(vettingImportTimeout)*time.Second, vettingPollInterval)
	},
}
