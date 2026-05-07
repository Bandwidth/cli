package bulk

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	createNumbersFile     string
	createNumbers         []string
	createCustomerOrderID string
	createIfNotExists     bool
	createSiteID          string
	createPeerID          string
	createFOCDate         string
)

func init() {
	createCmd.Flags().StringVar(&createNumbersFile, "numbers-file", "", "Path to a file with one TN per line (or comma-separated)")
	createCmd.Flags().StringSliceVar(&createNumbers, "numbers", nil, "TNs to port in, comma-separated or repeated. Either --numbers or --numbers-file is required.")
	createCmd.Flags().StringVar(&createSiteID, "site", "", "Site (sub-account) ID for the destination")
	createCmd.Flags().StringVar(&createPeerID, "peer", "", "SIP peer (location) ID for the destination")
	createCmd.Flags().StringVar(&createFOCDate, "foc", "", "Requested FOC date (ISO 8601)")
	createCmd.Flags().StringVar(&createCustomerOrderID, "customer-order-id", "", "Customer-supplied order identifier (used as the idempotency key)")
	createCmd.Flags().BoolVar(&createIfNotExists, "if-not-exists", false, "If a bulk port-in with the given --customer-order-id already exists, return it")
	Cmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create [flags]",
	Short: "Create a bulk port-in order with a large TN list",
	Long: `Submits a bulk port-in. The API splits the list across multiple child
port-in orders (one per RespOrg or carrier group) and validates each TN
asynchronously. Use ` + "`band portin bulk get-tns <id> --wait`" + ` to poll
the validation outcome.`,
	Example: `  band portin bulk create --numbers-file ./tns.txt --site 1234 --peer 5678
  band portin bulk create --numbers +18005551234,+18885551234 --foc 2026-06-01Z`,
	RunE: runCreate,
}

func runCreate(cmd *cobra.Command, args []string) error {
	tns, err := loadNumbers()
	if err != nil {
		return err
	}
	if len(tns) == 0 {
		return errors.New("no TNs supplied — use --numbers or --numbers-file")
	}

	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	if createIfNotExists {
		if createCustomerOrderID == "" {
			return errors.New("--if-not-exists requires --customer-order-id")
		}
		existing, err := findBulkByCustomerOrderID(client, acctID, createCustomerOrderID)
		if err != nil {
			return err
		}
		if existing != nil {
			return emitBulk(cmd, existing)
		}
	}

	body := map[string]interface{}{
		"ListOfPhoneNumbers": map[string]interface{}{
			"PhoneNumber": tns,
		},
	}
	if createSiteID != "" {
		body["SiteId"] = createSiteID
	}
	if createPeerID != "" {
		body["PeerId"] = createPeerID
	}
	if createFOCDate != "" {
		body["RequestedFocDate"] = createFOCDate
	}
	if createCustomerOrderID != "" {
		body["CustomerOrderId"] = createCustomerOrderID
	}

	var result interface{}
	if err := client.Post(
		fmt.Sprintf("/accounts/%s/bulkPortins", acctID),
		api.XMLBody{RootElement: "LnpOrder", Data: body},
		&result,
	); err != nil {
		return bulkError(err, "creating bulk port-in")
	}

	return emitBulk(cmd, result)
}

func loadNumbers() ([]string, error) {
	out := []string{}
	for _, n := range createNumbers {
		out = append(out, cmdutil.NormalizeNumber(n))
	}
	if createNumbersFile != "" {
		f, err := os.Open(createNumbersFile)
		if err != nil {
			return nil, fmt.Errorf("opening numbers file: %w", err)
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			for _, part := range strings.Split(line, ",") {
				if p := strings.TrimSpace(part); p != "" {
					out = append(out, cmdutil.NormalizeNumber(p))
				}
			}
		}
		if err := s.Err(); err != nil {
			return nil, fmt.Errorf("reading numbers file: %w", err)
		}
	}
	return out, nil
}

func findBulkByCustomerOrderID(client *api.Client, acctID, customerOrderID string) (interface{}, error) {
	q := url.Values{}
	q.Set("customerOrderId", customerOrderID)
	path := fmt.Sprintf("/accounts/%s/bulkPortins?%s", acctID, q.Encode())

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return nil, bulkError(err, "checking for existing bulk port-in by customer-order-id")
	}
	if digString(result, "CustomerOrderId") == customerOrderID {
		return result, nil
	}
	return nil, nil
}

func emitBulk(cmd *cobra.Command, result interface{}) error {
	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, flattenBulkResult(result))
	}
	return output.StdoutAuto(format, plain, result)
}
