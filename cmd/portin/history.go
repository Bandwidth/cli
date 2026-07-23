package portin

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(historyCmd)
}

var historyCmd = &cobra.Command{
	Use:     "history <order-id>",
	Short:   "Get the state-change history for a port-in order",
	Example: `  band portin history b9ef682b-2b42-4287-bfe4-ba03ec57cb07`,
	Args:    cobra.ExactArgs(1),
	RunE:    runHistory,
}

func runHistory(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	if err := client.Get(fmt.Sprintf("/accounts/%s/portins/%s/history", acctID, args[0]), &result); err != nil {
		return portinError(err, "getting port-in history")
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, flattenHistory(result))
	}
	return output.StdoutAuto(format, plain, result)
}

// flattenHistory walks the response and produces an array of
// {state, timestamp, actor} objects.
func flattenHistory(result interface{}) []map[string]interface{} {
	out := []map[string]interface{}{}
	walkHistoryEntries(result, &out)
	return out
}

func walkHistoryEntries(v interface{}, out *[]map[string]interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		// Heuristic: a history entry has at least a Date/Status pair.
		if _, hasStatus := val["Status"]; hasStatus {
			*out = append(*out, map[string]interface{}{
				"state":     digString(val, "Status"),
				"timestamp": digString(val, "Date"),
				"actor":     digString(val, "User"),
			})
			return
		}
		for _, child := range val {
			walkHistoryEntries(child, out)
		}
	case []interface{}:
		for _, item := range val {
			walkHistoryEntries(item, out)
		}
	}
}
