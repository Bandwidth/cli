package number

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	listStatus       string
	listNpaNxx       string
	listState        string
	listRateCenter   string
	listLata         string
	listSubaccount   string
	listLocation     string
	listDisconnected bool
)

func init() {
	listCmd.Flags().StringVar(&listStatus, "status", "Inservice",
		"Comma-separated statuses to include. Common values: Inservice (live), "+
			"InAccount (assigned, not yet live), Aging (released, in aging period).")
	listCmd.Flags().StringVar(&listNpaNxx, "npa-nxx", "", "Filter to a 6-digit NPA-NXX prefix (in-service numbers only; requires the inservice role)")
	listCmd.Flags().StringVar(&listState, "state", "", "Filter to a 2-letter state/province (in-service numbers only)")
	listCmd.Flags().StringVar(&listRateCenter, "ratecenter", "", "Filter to a rate center; requires --state")
	listCmd.Flags().StringVar(&listLata, "lata", "", "Filter to a LATA (in-service numbers only)")
	listCmd.Flags().StringVar(&listSubaccount, "subaccount", "", "List numbers on a sub-account (site ID)")
	listCmd.Flags().StringVar(&listLocation, "location", "", "List numbers on a location (SIP peer ID); requires --subaccount")
	listCmd.Flags().BoolVar(&listDisconnected, "disconnected", false, "List disconnected numbers instead of in-service ones")
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List phone numbers on the account",
	Long: `Lists phone numbers on the active account.

By default, returns only numbers in service (ready to route calls or send
messages). Pass --status to include numbers in other states.`,
	Example: `  band number list                                # default: only in-service
  band number list --status Inservice,InAccount   # include numbers just ordered
  band number list --status Aging                 # numbers being released
  band number list --npa-nxx 919555                # in-service numbers in an NPA-NXX
  band number list --state NC --ratecenter RALEIGH # in-service numbers in a rate center
  band number list --subaccount 407                # numbers on a sub-account
  band number list --subaccount 407 --location 500017
  band number list --disconnected                  # recently disconnected numbers`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	// --status has a default, so only treat it as user intent when changed;
	// otherwise the default value would conflict with every filter flag.
	opts := listOptions{
		NpaNxx:       listNpaNxx,
		State:        listState,
		RateCenter:   listRateCenter,
		Lata:         listLata,
		Subaccount:   listSubaccount,
		Location:     listLocation,
		Disconnected: listDisconnected,
	}
	if cmd.Flags().Changed("status") {
		opts.Status = listStatus
	}
	if err := opts.validate(); err != nil {
		return err
	}

	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var numbers []string
	if query := buildListQuery(acctID, opts); query != nil {
		numbers, err = fetchPagedNumbers(client, query)
	} else {
		numbers, err = fetchAccountNumbers(client, acctID, listStatus)
	}
	if err != nil {
		return err
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, numbers)
}

// tnsMaxPageSize is the largest page size /tns accepts. The endpoint rejects
// size > 2500 with error 1006. We paginate internally if an account has more.
const tnsMaxPageSize = 2500

// tnsMaxPages caps how many pages we'll fetch as a safety net. 2500 * 100 =
// 250k numbers is well beyond any realistic account size; exceeding this
// almost certainly means a bug or a broken server loop.
const tnsMaxPages = 100

// fetchAccountNumbers queries /tns for numbers on acctID matching the given
// comma-separated status filter, paginating as needed, and returns their
// FullNumbers formatted as E.164 strings. /tns is preferred over
// /accounts/{id}/inserviceNumbers because it's accessible to credentials
// without the inservice role.
func fetchAccountNumbers(client *api.Client, acctID, status string) ([]string, error) {
	var all []string
	for page := 1; page <= tnsMaxPages; page++ {
		q := url.Values{}
		q.Set("accountId", acctID)
		q.Set("status", status)
		q.Set("size", strconv.Itoa(tnsMaxPageSize))
		q.Set("page", strconv.Itoa(page))

		var result interface{}
		if err := client.Get("/tns?"+q.Encode(), &result); err != nil {
			return nil, wrapTNsError(err, acctID, cmdutil.ActiveBuild())
		}

		batch := extractFullNumbers(result)
		all = append(all, batch...)
		if len(batch) < tnsMaxPageSize {
			return all, nil
		}
	}
	return nil, fmt.Errorf("listing phone numbers: exceeded %d pages (%d numbers); "+
		"narrow the query with --status or contact support",
		tnsMaxPages, tnsMaxPages*tnsMaxPageSize)
}

// wrapTNsError annotates /tns errors with actionable context. /tns returns
// an empty body on 403, so the raw APIError message ("API error 403:") is
// not useful to the user. Build accounts get a tailored hint that their
// pre-provisioned number is reachable via the account portal; other
// accounts are pointed at the Numbers role. isBuild is parameterized so
// the function is testable without depending on a loaded config.
func wrapTNsError(err error, acctID string, isBuild bool) error {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 403 {
		return fmt.Errorf("listing phone numbers: %w", err)
	}
	if isBuild {
		return cmdutil.NewFeatureLimit(
			"phone number listing isn't available on Bandwidth Build accounts yet.\n"+
				"Your pre-provisioned number is visible in the Bandwidth account portal\n"+
				"and is already wired to the default voice application. Listing support\n"+
				"is planned for an upcoming Build update.", err)
	}
	return cmdutil.NewFeatureLimit(fmt.Sprintf(
		"listing phone numbers: credential lacks the Numbers role on account %s.\n"+
			"Contact your Bandwidth account manager to assign this role.", acctID), err)
}

// pagedListSize is the page size for the inserviceNumbers/discnumbers/site
// list endpoints. These endpoints document no maximum; 1000 keeps request
// counts low while staying well under any plausible server cap.
const pagedListSize = 1000

// fetchPagedNumbers pages through a filtered list endpoint and returns the
// merged E.164 numbers. The endpoints use 1-based page/size query params and
// signal the last page by returning fewer rows than requested.
func fetchPagedNumbers(client *api.Client, query *listQuery) ([]string, error) {
	var all []string
	for page := 1; page <= tnsMaxPages; page++ {
		q := url.Values{}
		for k, vs := range query.Query {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		q.Set("page", strconv.Itoa(page))
		q.Set("size", strconv.Itoa(pagedListSize))

		var result interface{}
		if err := client.Get(query.Path+"?"+q.Encode(), &result); err != nil {
			return nil, fmt.Errorf("listing phone numbers: %w", err)
		}

		batch := extractFullNumbers(result)
		all = append(all, batch...)
		if len(batch) < pagedListSize {
			return all, nil
		}
	}
	return nil, fmt.Errorf("listing phone numbers: exceeded %d pages (%d numbers); "+
		"narrow the query or contact support", tnsMaxPages, tnsMaxPages*pagedListSize)
}

// extractFullNumbers walks a decoded /tns response and returns each
// TelephoneNumber's FullNumber formatted as E.164.
func extractFullNumbers(raw interface{}) []string {
	var out []string
	collectFullNumbers(raw, &out)
	return out
}

func collectFullNumbers(v interface{}, out *[]string) {
	switch x := v.(type) {
	case map[string]interface{}:
		if fn, ok := x["FullNumber"].(string); ok && fn != "" {
			*out = append(*out, cmdutil.NormalizeE164(fn))
			return
		}
		// The inserviceNumbers and discnumbers endpoints return bare strings
		// under <TelephoneNumbers><TelephoneNumber>, not FullNumber objects.
		if tn, ok := x["TelephoneNumber"]; ok {
			if collectBareNumbers(tn, out) {
				return
			}
		}
		for _, child := range x {
			collectFullNumbers(child, out)
		}
	case []interface{}:
		for _, item := range x {
			collectFullNumbers(item, out)
		}
	}
}

// collectBareNumbers appends bare-string telephone numbers (a single string
// or a list of strings) and reports whether it consumed the value. Object
// forms of TelephoneNumber return false so the caller keeps walking.
func collectBareNumbers(v interface{}, out *[]string) bool {
	switch x := v.(type) {
	case string:
		if x != "" {
			*out = append(*out, cmdutil.NormalizeE164(x))
		}
		return true
	case []interface{}:
		consumed := false
		for _, item := range x {
			if s, ok := item.(string); ok {
				if s != "" {
					*out = append(*out, cmdutil.NormalizeE164(s))
				}
				consumed = true
			} else {
				collectFullNumbers(item, out)
				consumed = true
			}
		}
		return consumed
	}
	return false
}
