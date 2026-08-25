package tollfree

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

func init() {
	Cmd.AddCommand(templateCmd)
}

var templateCmd = &cobra.Command{
	Use:   "template <number> [number...]",
	Short: "Look up the routing template assigned to toll-free numbers",
	Long: `Returns the toll-free routing template name assigned to each number.

The template identifies how inbound calls to the number are routed at the
toll-free registry level. Numbers must be in-service on the account. Up to
5000 numbers per invocation.

This endpoint is gated per account: a 403 means toll-free template search
is not enabled — ask your Bandwidth account manager to enable it.`,
	Example: `  band tollfree template +18005551234
  band tollfree template 8005551234 8885551234 --plain`,
	Args: cobra.MinimumNArgs(1),
	RunE: runTemplate,
}

// maxTemplateNumbers is the API's documented per-request limit.
const maxTemplateNumbers = 5000

// nanpE164Re matches a full NANP number in E.164. ClassifyNumber checks only
// length and area code, so this guards against non-digit input (e.g. vanity
// letters) reaching the API.
var nanpE164Re = regexp.MustCompile(`^\+1\d{10}$`)

// normalizeTollFreeNumbers converts each argument to E.164, rejects numbers
// that are not NANP toll-free, and drops duplicates while preserving order.
func normalizeTollFreeNumbers(args []string) ([]string, error) {
	seen := make(map[string]bool, len(args))
	out := make([]string, 0, len(args))
	for _, a := range args {
		n := cmdutil.NormalizeE164(a)
		if !nanpE164Re.MatchString(n) || cmdutil.ClassifyNumber(n) != cmdutil.NumberTypeTollFree {
			return nil, cmdutil.NewFlagError(fmt.Sprintf("%s is not a toll-free number (toll-free prefixes: 800, 888, 877, 866, 855, 844, 833; digits only)", a))
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	if len(out) > maxTemplateNumbers {
		return nil, cmdutil.NewFlagError(fmt.Sprintf("too many numbers: %d (the API accepts at most %d per request)", len(out), maxTemplateNumbers))
	}
	return out, nil
}

// templateSearchBody builds the request payload for the template search
// endpoint, which accepts exactly one IN criterion on phoneNumbers.
func templateSearchBody(numbers []string) map[string]interface{} {
	return map[string]interface{}{
		"queryCriteria": []map[string]interface{}{{
			"operator":  "IN",
			"parameter": "phoneNumbers",
			"values":    numbers,
		}},
	}
}

// unwrapTemplateMappings extracts data.phoneNumberTemplateMappings from the
// response. If the shape is unexpected, the raw response is returned so the
// user still sees what the server said.
func unwrapTemplateMappings(result interface{}) interface{} {
	m, ok := result.(map[string]interface{})
	if !ok {
		return result
	}
	data, ok := m["data"].(map[string]interface{})
	if !ok {
		return result
	}
	if mappings, ok := data["phoneNumberTemplateMappings"]; ok {
		return mappings
	}
	return result
}

func runTemplate(cmd *cobra.Command, args []string) error {
	numbers, err := normalizeTollFreeNumbers(args)
	if err != nil {
		return err
	}

	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v2/accounts/%s/tollFreeTemplateAssignments/search", acctID)
	var result interface{}
	if err := client.Post(cmd.Context(), path, templateSearchBody(numbers), &result); err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 403 {
			return fmt.Errorf("toll-free template search is not enabled on account %s — ask your Bandwidth account manager to enable it: %w", acctID, err)
		}
		return fmt.Errorf("searching toll-free template assignments: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	return output.StdoutAuto(format, plain, unwrapTemplateMappings(result))
}
