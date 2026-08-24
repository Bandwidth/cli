package insights

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

// monitorFlags are the filters shared by every monitor subcommand. The
// Insights API encodes them deepObject-style (e.g. timestamp[gte]=...).
type monitorFlags struct {
	To         string
	From       string
	Direction  string
	CallType   string
	Subaccount string
	Since      string
	Until      string
}

// monitor describes one /v1/monitors/voice endpoint exposed as a subcommand.
type monitor struct {
	use   string // subcommand name; matches the API path segment
	short string
}

// monitors are the v1 set: the aggregates that together describe a number's
// or account's traffic profile. The API's other monitors (calls-per-second,
// concurrent-calls, error-percentages, network-efficiency-ratios,
// short-calls, call-data) can be added to this table as needed.
var monitors = []monitor{
	{use: "minutes-of-use", short: "Aggregated minutes of use per time slice"},
	{use: "completed-calls", short: "Completed call counts per time slice"},
	{use: "failed-calls", short: "Failed call counts per time slice"},
	{use: "connection-rates", short: "Call connection rates per time slice"},
	{use: "average-durations", short: "Average call durations per time slice"},
}

func init() {
	for _, m := range monitors {
		Cmd.AddCommand(newMonitorCmd(m))
	}
}

func newMonitorCmd(m monitor) *cobra.Command {
	flags := &monitorFlags{}
	cmd := &cobra.Command{
		Use:   m.use,
		Short: m.short,
		Long: m.short + `.

All filters are optional; unfiltered requests cover the whole account over
the last 7 days. Phone-number filters accept comma-separated E.164 values
and can be slow on large accounts — add other filters to narrow the scope.`,
		Example: fmt.Sprintf(`  band insights %[1]s
  band insights %[1]s --to +18005551234 --since 30d
  band insights %[1]s --call-type TOLLFREE-IN --direction INBOUND --since 2026-07-01T00:00:00Z`, m.use),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMonitor(cmd, m.use, flags)
		},
	}
	cmd.Flags().StringVar(&flags.To, "to", "", "Filter by destination number(s), comma-separated E.164")
	cmd.Flags().StringVar(&flags.From, "from", "", "Filter by originating number(s), comma-separated E.164")
	cmd.Flags().StringVar(&flags.Direction, "direction", "", "Filter by call direction: INBOUND or OUTBOUND")
	cmd.Flags().StringVar(&flags.CallType, "call-type", "", "Filter by call type (e.g. TOLLFREE-IN, TOLLFREE-OUT, LOCAL, INTERSTATE)")
	cmd.Flags().StringVar(&flags.Subaccount, "subaccount", "", "Filter by sub-account ID")
	cmd.Flags().StringVar(&flags.Since, "since", "", "Start of the time range: RFC3339 or relative (e.g. 30d, 24h, 90m); default 7 days ago")
	cmd.Flags().StringVar(&flags.Until, "until", "", "End of the time range: RFC3339 or relative; default now")
	return cmd
}

// relativeTimeRe matches the relative time shorthand: <n>d, <n>h, or <n>m.
var relativeTimeRe = regexp.MustCompile(`^(\d+)([dhm])$`)

// parseTimeFlag converts a --since/--until value to RFC3339. Relative values
// are anchored at now; RFC3339 values pass through verbatim (the API handles
// timezone interpretation).
func parseTimeFlag(value string, now time.Time) (string, error) {
	if m := relativeTimeRe.FindStringSubmatch(value); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return "", cmdutil.NewFlagError(fmt.Sprintf("invalid relative time %q", value))
		}
		var unit time.Duration
		switch m[2] {
		case "d":
			unit = 24 * time.Hour
		case "h":
			unit = time.Hour
		case "m":
			unit = time.Minute
		}
		return now.Add(-time.Duration(n) * unit).UTC().Format(time.RFC3339), nil
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return "", cmdutil.NewFlagError(fmt.Sprintf("invalid time %q: use RFC3339 (2026-07-01T00:00:00Z) or relative shorthand (30d, 24h, 90m)", value))
	}
	return value, nil
}

// normalizeCallType uppercases and converts dashes to underscores: the query
// filter enum uses TOLLFREE_IN while responses (and Bandwidth docs) render
// TOLLFREE-IN, so accept either form.
func normalizeCallType(v string) string {
	return strings.ReplaceAll(strings.ToUpper(v), "-", "_")
}

// buildMonitorQuery renders the deepObject query parameters for a monitor
// request. accountId[eq] is required by the API and always present.
func buildMonitorQuery(acctID string, f monitorFlags, now time.Time) (url.Values, error) {
	q := url.Values{}
	q.Set("accountId[eq]", acctID)
	if f.To != "" {
		q.Set("toPhoneNumber[eq]", f.To)
	}
	if f.From != "" {
		q.Set("fromPhoneNumber[eq]", f.From)
	}
	if f.Direction != "" {
		d := strings.ToUpper(f.Direction)
		if d != "INBOUND" && d != "OUTBOUND" {
			return nil, cmdutil.NewFlagError(fmt.Sprintf("invalid --direction %q: use INBOUND or OUTBOUND", f.Direction))
		}
		q.Set("direction[eq]", d)
	}
	if f.CallType != "" {
		q.Set("callType[eq]", normalizeCallType(f.CallType))
	}
	if f.Subaccount != "" {
		q.Set("subAccount[eq]", f.Subaccount)
	}
	if f.Since != "" {
		ts, err := parseTimeFlag(f.Since, now)
		if err != nil {
			return nil, err
		}
		q.Set("timestamp[gte]", ts)
	}
	if f.Until != "" {
		ts, err := parseTimeFlag(f.Until, now)
		if err != nil {
			return nil, err
		}
		q.Set("timestamp[lte]", ts)
	}
	return q, nil
}

// unwrapMonitorData extracts the data object from the {links, data, errors}
// envelope. Unexpected shapes pass through raw.
func unwrapMonitorData(result interface{}) interface{} {
	if m, ok := result.(map[string]interface{}); ok {
		if data, ok := m["data"]; ok && data != nil {
			return data
		}
	}
	return result
}

func runMonitor(cmd *cobra.Command, endpoint string, flags *monitorFlags) error {
	client, acctID, err := cmdutil.InsightsClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	q, err := buildMonitorQuery(acctID, *flags, time.Now())
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get("/v1/monitors/voice/"+endpoint+"?"+q.Encode(), &result); err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
			return fmt.Errorf("the Monitoring API feature is not enabled on account %s — ask your Bandwidth account manager to enable it: %w", acctID, err)
		}
		return fmt.Errorf("getting %s: %w", endpoint, err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, unwrapMonitorData(result))
}
