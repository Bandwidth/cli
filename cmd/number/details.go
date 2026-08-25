package number

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(detailsCmd)
}

var detailsCmd = &cobra.Command{
	Use:   "details <number>",
	Short: "Get the full Dashboard view of a phone number",
	Long: `Returns everything the Bandwidth Dashboard knows about a phone number:
geography (LATA, state, rate center), vendor, sub-account and location,
service types, features (E911, LIDB, DLDA), messaging settings including
the assigned NN route, TN attributes, and — where configured — the
per-number origination route plan with priority and weight per endpoint.

This is the Dashboard (legacy platform) view and works for any number on
the account. For the Universal Platform voice record (VCP assignment),
use "band number get" instead.`,
	Example: `  band number details +19195551234
  band number details 8005551234 --plain`,
	Args: cobra.ExactArgs(1),
	RunE: runDetails,
}

// unwrapTelephoneNumberDetails strips the TelephoneNumberResponse envelope so
// the useful fields sit at the top level. Unexpected shapes pass through.
func unwrapTelephoneNumberDetails(result interface{}) interface{} {
	m, ok := result.(map[string]interface{})
	if !ok {
		return result
	}
	resp, ok := m["TelephoneNumberResponse"].(map[string]interface{})
	if !ok {
		resp = m
	}
	if details, ok := resp["TelephoneNumberDetails"]; ok {
		return details
	}
	return result
}

func runDetails(cmd *cobra.Command, args []string) error {
	number := cmdutil.NormalizeE164(args[0])

	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(cmd.Context(), fmt.Sprintf("/tns/%s/tndetails", number), &result); err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			// Keep the APIError wrapped so the 404 still maps to exit 3.
			return fmt.Errorf("getting number details: %s not found on account %s: %w", number, acctID, err)
		}
		return fmt.Errorf("getting number details: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, unwrapTelephoneNumberDetails(result))
}
