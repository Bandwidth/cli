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
	createCmd.Flags().StringVar(&createFOCDate, "foc", "", "Requested FOC date for the bulk order as a whole (YYYY-MM-DD)")
	createCmd.Flags().StringVar(&createCustomerOrderID, "customer-order-id", "", "Customer-supplied order identifier (used as the idempotency key)")
	createCmd.Flags().BoolVar(&createIfNotExists, "if-not-exists", false, "If a bulk port-in with the given --customer-order-id already exists, return it")
	Cmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create [flags]",
	Short: "Create a bulk port-in order with a large TN list",
	Long: `Submits a bulk port-in. This is a two-step call under the hood: the API
requires an empty template order first (POST /bulkPortins), then the TN
list on a second request (PUT /bulkPortins/{id}/tnList). The API then
splits the list across multiple child port-in orders (one per RespOrg or
carrier group) and validates each TN asynchronously. Use
` + "`band portin bulk get-tns <id> --wait`" + ` to poll the validation outcome.`,
	Example: `  band portin bulk create --numbers-file ./tns.txt --site 1234 --peer 5678
  band portin bulk create --numbers +18005551234,+18885551234 --foc 2026-06-01`,
	RunE: runCreate,
}

func runCreate(cmd *cobra.Command, args []string) error {
	tns, err := loadNumbers(createNumbers, createNumbersFile)
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
			if strings.ToUpper(digString(existing, "ProcessingStatus")) == "DRAFT" {
				// A DRAFT with no TN list means step 2 never completed —
				// attach the TN list to the stranded template instead of
				// returning it as-is.
				orderID := digString(existing, "OrderId")
				if orderID != "" {
					result, err := putTnList(client, acctID, orderID, tns)
					if err != nil {
						return err
					}
					return emitBulk(cmd, result)
				}
			}
			return emitBulk(cmd, existing)
		}
	}

	// Step 1: create the bulk template order. The bulk create endpoint does
	// NOT accept a TN list — it takes only fields to cascade onto the child
	// port-in orders, and rejects anything else (often as a bare 400 with no
	// payload). Numbers are attached in step 2 via the /tnList endpoint.
	body := map[string]interface{}{}
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

	var created interface{}
	if err := client.Post(
		fmt.Sprintf("/accounts/%s/bulkPortins", acctID),
		api.XMLBody{RootElement: "BulkPortin", Data: body},
		&created,
	); err != nil {
		return bulkError(err, "creating bulk port-in")
	}

	orderID := digString(created, "OrderId")
	if orderID == "" {
		return errors.New("creating bulk port-in: API response did not include an OrderId")
	}

	// Step 2: attach the TN list. If this fails, the template order already
	// exists — surface its ID so a retry doesn't strand it. Unsubmitted
	// drafts are auto-removed by the API after 2 days.
	result, err := putTnList(client, acctID, orderID, tns)
	if err != nil {
		resume := "re-run the same create to start over (unsubmitted drafts expire after 2 days)"
		if createCustomerOrderID != "" {
			resume = fmt.Sprintf("re-run the same create with --customer-order-id %s --if-not-exists to resume attaching the TN list", createCustomerOrderID)
		}
		return fmt.Errorf("bulk port-in created (id: %s) but adding the TN list failed — %s\n  underlying error: %w",
			orderID, resume, err)
	}

	return emitBulk(cmd, result)
}

// putTnList attaches a TN list to a bulk port-in template order via
// PUT /bulkPortins/{orderID}/tnList.
func putTnList(client *api.Client, acctID, orderID string, tns []string) (interface{}, error) {
	var result interface{}
	if err := client.Put(
		fmt.Sprintf("/accounts/%s/bulkPortins/%s/tnList", acctID, orderID),
		api.XMLBody{RootElement: "TnList", Data: map[string]interface{}{"TN": tns}},
		&result,
	); err != nil {
		return nil, bulkError(err, "adding TN list")
	}
	return result, nil
}

// loadNumbers merges --numbers and --numbers-file input into E.164 form.
// The /tnList endpoint rejects bare 10-digit values with error 1022
// ("Retry request with all E.164 formatted phone numbers") — observed live
// against prod, contradicting the 10-digit guidance in the API spec's 400
// example.
func loadNumbers(numbers []string, numbersFile string) ([]string, error) {
	out := []string{}
	for _, n := range numbers {
		out = append(out, cmdutil.NormalizeE164(n))
	}
	if numbersFile != "" {
		f, err := os.Open(numbersFile)
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
					out = append(out, cmdutil.NormalizeE164(p))
				}
			}
		}
		if err := s.Err(); err != nil {
			return nil, fmt.Errorf("reading numbers file: %w", err)
		}
	}
	return out, nil
}

// findBulkByCustomerOrderID returns the existing bulk port-in entry matching
// the given customer order ID, or nil if none exists.
//
// GET /bulkPortins does not support a customerOrderId query parameter
// (verified against the spec and live), and it excludes draft orders unless
// a status filter is passed — the same quirk as the single-order search in
// cmd/portin/create.go. So we page through each status with
// orderDetails=true, which makes each entry carry its CustomerOrderId, and
// match client-side.
//
// The spec claims status=draft covers all draft-family values
// (VALIDATE/VALID/INVALID_DRAFT_TNS); live it matches only literal DRAFT,
// but the endpoint accepts the specific draft-family values even though its
// documented enum omits them — both verified against prod. Draft states
// first: they're what an idempotent retry most often targets.
func findBulkByCustomerOrderID(client *api.Client, acctID, customerOrderID string) (interface{}, error) {
	statuses := []string{
		"draft",
		"validate_draft_tns",
		"valid_draft_tns",
		"invalid_draft_tns",
		"in_progress",
		"needs_attention",
		"partial",
		"completed",
		"cancelled",
	}
	for _, status := range statuses {
		q := url.Values{}
		q.Set("status", status)
		q.Set("page", "1")
		q.Set("size", "1000")
		q.Set("orderDetails", "true")
		path := fmt.Sprintf("/accounts/%s/bulkPortins?%s", acctID, q.Encode())

		var result interface{}
		if err := client.Get(path, &result); err != nil {
			var apiErr *api.APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
				continue
			}
			return nil, bulkError(err, "checking for existing bulk port-in by customer-order-id")
		}
		if entry := findBulkEntry(result, customerOrderID); entry != nil {
			return entry, nil
		}
	}
	return nil, nil
}

// findBulkEntry walks a BulkPortinResponses list payload and returns the
// entry whose CustomerOrderId matches, or nil. Matching per-entry matters:
// a global digString over the whole list would compare against whichever
// order happens to appear first.
func findBulkEntry(v interface{}, customerOrderID string) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		if entries, ok := val["BulkPortinResponse"]; ok {
			for _, entry := range asSlice(entries) {
				if digString(entry, "CustomerOrderId") == customerOrderID {
					return entry
				}
			}
		}
		for _, child := range val {
			if found := findBulkEntry(child, customerOrderID); found != nil {
				return found
			}
		}
	case []interface{}:
		for _, item := range val {
			if found := findBulkEntry(item, customerOrderID); found != nil {
				return found
			}
		}
	}
	return nil
}

// asSlice normalizes the XML-decoder ambiguity where a repeated element
// arrives as a single map when there is exactly one occurrence.
func asSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	return []interface{}{v}
}

func emitBulk(cmd *cobra.Command, result interface{}) error {
	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, flattenBulkResult(result))
	}
	return output.StdoutAuto(format, plain, result)
}
