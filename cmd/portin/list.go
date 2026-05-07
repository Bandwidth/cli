package portin

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	listStatus string
	listFrom   string
	listTo     string
)

func init() {
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by order status (DRAFT, SUBMITTED, FOC, COMPLETE, CANCELLED, etc.)")
	listCmd.Flags().StringVar(&listFrom, "from", "", "Modified date lower bound (ISO 8601)")
	listCmd.Flags().StringVar(&listTo, "to", "", "Modified date upper bound (ISO 8601)")
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List port-in orders on the active account",
	Example: `  band portin list
  band portin list --status SUBMITTED
  band portin list --from 2026-01-01T00:00:00Z --to 2026-04-01T00:00:00Z`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	params := url.Values{}
	if listStatus != "" {
		params.Set("status", listStatus)
	}
	if listFrom != "" {
		params.Set("modifiedDateFrom", listFrom)
	}
	if listTo != "" {
		params.Set("modifiedDateTo", listTo)
	}

	path := fmt.Sprintf("/accounts/%s/portins", acctID)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return portinError(err, "listing port-in orders")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		flat := flattenPortInList(result)
		return output.StdoutAuto(format, plain, flat)
	}
	return output.StdoutAuto(format, plain, result)
}

// flattenPortInList walks a list response and produces an array of the
// stable plain shape, even when the API returns a single-element object
// instead of a list.
func flattenPortInList(result interface{}) []map[string]interface{} {
	out := []map[string]interface{}{}
	// Look for PortInOrder entries anywhere in the response.
	walkPortInOrders(result, &out)
	return out
}

func walkPortInOrders(v interface{}, out *[]map[string]interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		// If this map has an OrderId, treat it as a single port-in.
		if _, has := val["OrderId"]; has {
			*out = append(*out, flattenPortInResult(val, ""))
			return
		}
		for _, child := range val {
			walkPortInOrders(child, out)
		}
	case []interface{}:
		for _, item := range val {
			walkPortInOrders(item, out)
		}
	}
}
