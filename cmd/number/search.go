package number

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	searchAreaCode string
	searchQuantity string
)

func init() {
	searchCmd.Flags().StringVar(&searchAreaCode, "area-code", "", "Area code to search (required)")
	searchCmd.Flags().StringVar(&searchQuantity, "quantity", "10", "Number of results to return")
	_ = searchCmd.MarkFlagRequired("area-code")
	Cmd.AddCommand(searchCmd)
}

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search available phone numbers",
	Long:  "Searches for available phone numbers that can be ordered. Results are not reserved — order promptly.",
	Example: `  # Search by area code
  band number search --area-code 919

  # Limit results
  band number search --area-code 704 --quantity 3

  # Agent-friendly: get just the numbers
  band number search --area-code 919 --plain`,
	RunE: runSearch,
}

func runSearch(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	q := url.Values{}
	q.Set("areaCode", searchAreaCode)
	q.Set("quantity", searchQuantity)

	var result interface{}
	path := fmt.Sprintf("/accounts/%s/availableNumbers?%s", acctID, q.Encode())
	if err := client.Get(path, &result); err != nil {
		return fmt.Errorf("searching available numbers: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutPlainList(format, plain, result)
}
