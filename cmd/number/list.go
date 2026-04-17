package number

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var listStatus string

func init() {
	listCmd.Flags().StringVar(&listStatus, "status", "Inservice",
		"Comma-separated statuses to include. Common values: Inservice (live), "+
			"InAccount (assigned, not yet live), Aging (released, in aging period).")
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List phone numbers on the account",
	Long: `Lists phone numbers on the active account.

By default, returns only numbers in service (ready to route calls or send
messages). Pass --status to include numbers in other states.

Examples:
  band number list                                # default: only in-service
  band number list --status Inservice,InAccount   # include numbers just ordered
  band number list --status Aging                 # numbers being released`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	numbers, err := fetchAccountNumbers(client, acctID, listStatus)
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
			return nil, wrapTNsError(err, acctID)
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

// wrapTNsError annotates /tns errors with actionable context. The endpoint
// returns an empty body on 403, so the raw APIError message is just
// "API error 403:" — not useful to the user. The most common 403 cases are
// Build (express) credentials, which don't yet include the Numbers role
// (coming in a future Build update), and regular credentials missing the role.
func wrapTNsError(err error, acctID string) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
		return fmt.Errorf("listing phone numbers: credential lacks the Numbers role on account %s.\n"+
			"Build credentials don't include this role yet — it'll be added in an upcoming\n"+
			"Build update. In the meantime, your pre-provisioned number is visible in the\n"+
			"Bandwidth account portal and is already wired to the default voice application.\n"+
			"For non-Build accounts, contact your Bandwidth account manager to grant the\n"+
			"Numbers role: %w", acctID, err)
	}
	return fmt.Errorf("listing phone numbers: %w", err)
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
			*out = append(*out, normalizeE164(fn))
			return
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

// normalizeE164 returns n in E.164 form. /tns may return either a 10-digit
// US number ("9195551234") or an already-prefixed value depending on whether
// the account is on the v2 E.164 response format.
func normalizeE164(n string) string {
	if strings.HasPrefix(n, "+") {
		return n
	}
	if len(n) == 10 {
		return "+1" + n
	}
	return "+" + n
}
