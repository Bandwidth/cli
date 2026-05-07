package portin

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	createNumbers              []string
	createLoaPath              string
	createSiteID               string
	createPeerID               string
	createFOCDate              string
	createLoaAuthorizingPerson string
	createCustomerOrderID      string
	createIfNotExists          bool
)

func init() {
	createCmd.Flags().StringSliceVar(&createNumbers, "numbers", nil, "Telephone numbers to port in, comma-separated or repeated (required)")
	createCmd.Flags().StringVar(&createLoaPath, "loa", "", "Path to an LOA or supporting document to upload alongside the order")
	createCmd.Flags().StringVar(&createSiteID, "site", "", "Site (sub-account) ID for the destination")
	createCmd.Flags().StringVar(&createPeerID, "peer", "", "SIP peer (location) ID for the destination")
	createCmd.Flags().StringVar(&createFOCDate, "foc", "", "Requested FOC date (ISO 8601 — e.g. 2026-06-01Z)")
	createCmd.Flags().StringVar(&createLoaAuthorizingPerson, "loa-authorizing-person", "", "Name of the person authorizing the LOA")
	createCmd.Flags().StringVar(&createCustomerOrderID, "customer-order-id", "", "Customer-supplied order identifier (used as the idempotency key with --if-not-exists)")
	createCmd.Flags().BoolVar(&createIfNotExists, "if-not-exists", false, "If a port-in with the given --customer-order-id already exists, return it instead of creating a new one")
	_ = createCmd.MarkFlagRequired("numbers")
	Cmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create --numbers <...> [flags]",
	Short: "Create a draft port-in order",
	Long: `Creates a port-in order in DRAFT state. With --loa, also uploads a
supporting document in the same command. To send the order on to Neustar /
SOMOS, follow up with: band portin submit <order-id>.

For idempotency in agent retry loops, pass --customer-order-id <id> with
--if-not-exists. On retry, an existing order with the same customer ID is
returned instead of creating a duplicate.`,
	Example: `  band portin create --numbers +19195551234 --site 1234 --peer 5678 \
    --foc 2026-06-01Z --loa-authorizing-person "Jane Doe" --loa ./loa.pdf
  band portin create --numbers +19195551234 --customer-order-id agent-run-42 --if-not-exists`,
	RunE: runCreate,
}

func runCreate(cmd *cobra.Command, args []string) error {
	if len(createNumbers) == 0 {
		return errors.New("--numbers is required")
	}

	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	// Idempotency: if --if-not-exists is set with --customer-order-id, look up
	// any existing order with that customer ID before creating a new one.
	if createIfNotExists {
		if createCustomerOrderID == "" {
			return errors.New("--if-not-exists requires --customer-order-id")
		}
		existing, err := findByCustomerOrderID(client, acctID, createCustomerOrderID)
		if err != nil {
			return err
		}
		if existing != nil {
			return emit(cmd, existing)
		}
	}

	numbers := make([]string, len(createNumbers))
	for i, n := range createNumbers {
		numbers[i] = stripE164(cmdutil.NormalizeNumber(n))
	}

	body := map[string]interface{}{
		"ListOfPhoneNumbers": map[string]interface{}{
			"PhoneNumber": numbers,
		},
		"ProcessingStatus": "DRAFT",
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
	if createLoaAuthorizingPerson != "" {
		body["LoaAuthorizingPerson"] = createLoaAuthorizingPerson
	}
	if createCustomerOrderID != "" {
		body["CustomerOrderId"] = createCustomerOrderID
	}

	var result interface{}
	if err := client.Post(
		fmt.Sprintf("/accounts/%s/portins", acctID),
		api.XMLBody{RootElement: "LnpOrder", Data: body},
		&result,
	); err != nil {
		return portinError(err, "creating port-in order")
	}

	orderID := digString(result, "OrderId")

	// Optional LOA upload chained onto the create.
	if createLoaPath != "" && orderID != "" {
		data, err := os.ReadFile(createLoaPath)
		if err != nil {
			return fmt.Errorf("reading LOA file: %w", err)
		}
		ct := detectContentType(createLoaPath)
		path := fmt.Sprintf("/accounts/%s/portins/%s/loas", acctID, orderID)
		if _, err := client.PostMultipart(path, "loaFile", filepath.Base(createLoaPath), data, ct); err != nil {
			return portinError(err, "uploading LOA")
		}
	}

	return emit(cmd, result)
}

// findByCustomerOrderID returns the existing port-in order matching the given
// customer order ID, or nil if none exists. Errors only on hard API failures.
func findByCustomerOrderID(client *api.Client, acctID, customerOrderID string) (interface{}, error) {
	q := url.Values{}
	q.Set("customerOrderId", customerOrderID)
	path := fmt.Sprintf("/accounts/%s/portins?%s", acctID, q.Encode())

	var result interface{}
	if err := client.Get(path, &result); err != nil {
		return nil, portinError(err, "checking for existing port-in by customer-order-id")
	}

	// Walk the response for an order whose CustomerOrderId matches exactly.
	flat := flattenPortInList(result)
	for _, o := range flat {
		if id, _ := o["customerOrderId"].(string); id == customerOrderID {
			return result, nil
		}
	}
	return nil, nil
}

func emit(cmd *cobra.Command, result interface{}) error {
	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		return output.StdoutAuto(format, plain, flattenPortInResult(result))
	}
	return output.StdoutAuto(format, plain, result)
}
