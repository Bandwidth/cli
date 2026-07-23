package bulk

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	listStatus       string
	listOrderDate    string
	listFrom         string
	listTo           string
	listPage         string
	listSize         int
	listOrderDetails bool
)

func init() {
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by status (draft, in_progress, needs_attention, partial, completed, cancelled)")
	listCmd.Flags().StringVar(&listOrderDate, "order-date", "", "Filter by a specific modification date (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listFrom, "from", "", "Modified-date lower bound (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listTo, "to", "", "Modified-date upper bound (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listPage, "page", "1", "Page (orderId of first order on the page, or '1' for the first page)")
	listCmd.Flags().IntVar(&listSize, "size", 30, "Page size (1-1000)")
	listCmd.Flags().BoolVar(&listOrderDetails, "order-details", false, "Include full order details instead of summary entries")
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List bulk port-in orders",
	Long: `Lists bulk port-in orders. Pagination is mandatory on the API side —
defaults are page=1 size=30. Without --from/--to, the API returns orders
modified within the last two years.`,
	Example: `  band portin bulk list
  band portin bulk list --status draft
  band portin bulk list --from 2026-01-01 --to 2026-04-01 --order-details`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	q := url.Values{}
	q.Set("page", listPage)
	q.Set("size", fmt.Sprintf("%d", listSize))
	if listStatus != "" {
		q.Set("status", listStatus)
	}
	if listOrderDate != "" {
		q.Set("orderDate", listOrderDate)
	}
	if listFrom != "" {
		q.Set("modifiedDateFrom", listFrom)
	}
	if listTo != "" {
		q.Set("modifiedDateTo", listTo)
	}
	if listOrderDetails {
		q.Set("orderDetails", "true")
	}

	path := fmt.Sprintf("/accounts/%s/bulkPortins?%s", acctID, q.Encode())

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return bulkError(err, "listing bulk port-in orders")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
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
