// Package insights implements `band insights`, read-only voice usage and
// quality aggregates from the Bandwidth Insights Monitoring API.
package insights

import "github.com/spf13/cobra"

// Cmd is the `band insights` parent command.
var Cmd = &cobra.Command{
	Use:   "insights",
	Short: "Voice usage and quality aggregates (minutes of use, call counts, connection rates)",
	Long: `Read aggregated voice traffic data from the Bandwidth Insights API:
minutes of use, completed and failed calls, connection rates, and average
call durations — filterable by phone number, direction, call type, and
sub-account.

Results are broken into time slices whose granularity scales with the
requested window (hours for a few days, months for long ranges). History
goes back at most one year. Defaults to the last 7 days when no time range
is given.

Requires the Monitoring API feature on your account. If you get a 403
error, ask your Bandwidth account manager to enable it.`,
}
