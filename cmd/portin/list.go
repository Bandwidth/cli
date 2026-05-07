package portin

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	listStatus          string
	listStartDate       string
	listEndDate         string
	listTN              string
	listOrderTN         string
	listCustomerOrderID string
	listPON             string
	listPage            int
	listSize            int
)

func init() {
	listCmd.Flags().StringVar(&listStatus, "status", "", "Filter by order status (DRAFT, SUBMITTED, FOC, COMPLETE, CANCELLED, etc.)")
	listCmd.Flags().StringVar(&listStartDate, "start-date", "", "Earliest last-modified date (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listEndDate, "end-date", "", "Latest last-modified date (YYYY-MM-DD)")
	listCmd.Flags().StringVar(&listTN, "tn", "", "Filter by billing TN")
	listCmd.Flags().StringVar(&listOrderTN, "order-tn", "", "Filter by one of the TNs being ported")
	listCmd.Flags().StringVar(&listCustomerOrderID, "customer-order-id", "", "Filter by customer-supplied order ID")
	listCmd.Flags().StringVar(&listPON, "pon", "", "Filter by PON (purchase order number)")
	listCmd.Flags().IntVar(&listPage, "page", 1, "Page number (pagination)")
	listCmd.Flags().IntVar(&listSize, "size", 30, "Page size (pagination)")
	Cmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List port-in orders on the active account",
	Long: `Searches port-in orders on the active account. Pagination is mandatory
on the API side — defaults are page=1 size=30. Filters are AND-ed.`,
	Example: `  band portin list
  band portin list --status SUBMITTED --size 100
  band portin list --start-date 2026-01-01 --end-date 2026-04-01
  band portin list --customer-order-id agent-run-42`,
	RunE: runList,
}

func runList(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	params := url.Values{}
	// page and size are documented as required by the Numbers API.
	params.Set("page", fmt.Sprintf("%d", listPage))
	params.Set("size", fmt.Sprintf("%d", listSize))
	if listStatus != "" {
		params.Set("status", listStatus)
	}
	if listStartDate != "" {
		params.Set("startdate", listStartDate)
	}
	if listEndDate != "" {
		params.Set("enddate", listEndDate)
	}
	if listTN != "" {
		params.Set("tn", listTN)
	}
	if listOrderTN != "" {
		params.Set("orderTn", listOrderTN)
	}
	if listCustomerOrderID != "" {
		params.Set("customerOrderId", listCustomerOrderID)
	}
	if listPON != "" {
		params.Set("pon", listPON)
	}

	path := fmt.Sprintf("/accounts/%s/portins?%s", acctID, params.Encode())

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
	walkPortInOrders(result, &out)
	return out
}

func walkPortInOrders(v interface{}, out *[]map[string]interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
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
