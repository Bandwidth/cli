package bulk

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
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status (draft, in_progress, needs_attention, partial, completed, cancelled)")
	listCmd.Flags().StringVar(&listFrom, "from", "", "Modified date lower bound (ISO 8601)")
	listCmd.Flags().StringVar(&listTo, "to", "", "Modified date upper bound (ISO 8601)")
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List bulk port-in orders",
	Example: `  band portin bulk list
  band portin bulk list --status in_progress`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	q := url.Values{}
	if listStatus != "" {
		q.Set("status", listStatus)
	}
	if listFrom != "" {
		q.Set("modifiedDateFrom", listFrom)
	}
	if listTo != "" {
		q.Set("modifiedDateTo", listTo)
	}

	path := fmt.Sprintf("/accounts/%s/bulkPortins", acctID)
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return bulkError(err, "listing bulk port-in orders")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		// Walk and flatten each bulk order in the list.
		flat := []map[string]interface{}{}
		walkBulkOrders(result, &flat)
		return output.StdoutAuto(format, plain, flat)
	}
	return output.StdoutAuto(format, plain, result)
}

func walkBulkOrders(v interface{}, out *[]map[string]interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		if _, has := val["OrderId"]; has {
			*out = append(*out, flattenBulkResult(val))
			return
		}
		for _, child := range val {
			walkBulkOrders(child, out)
		}
	case []interface{}:
		for _, item := range val {
			walkBulkOrders(item, out)
		}
	}
}
