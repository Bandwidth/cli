package portin

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
)

var (
	validateTFWait    bool
	validateTFTimeout time.Duration
)

func init() {
	validateTFCmd.Flags().BoolVar(&validateTFWait, "wait", false, "Wait until validation reaches a terminal state (COMPLETE or FAILED)")
	validateTFCmd.Flags().DurationVar(&validateTFTimeout, "timeout", 60*time.Second, "Maximum time to wait (default 60s)")
	Cmd.AddCommand(validateTFCmd)
}

var validateTFCmd = &cobra.Command{
	Use:   "validate-tf <number> [number...]",
	Short: "Check whether toll-free numbers can be ported",
	Long: `Submits a toll-free porting validation order and reports portability
per number. Without --wait, returns the order in PROCESSING state and the
caller polls separately. With --wait, blocks until the order reaches COMPLETE
or FAILED.

When any number reports portable=false, this exits 1 with the per-number
reason — the negative result is surfaced rather than buried in the response.`,
	Example: `  band portin validate-tf +18005551234
  band portin validate-tf +18005551234 +18885551234 --wait --plain`,
	Args: cobra.MinimumNArgs(1),
	RunE: runValidateTF,
}

func runValidateTF(cmd *cobra.Command, args []string) error {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	tns := make([]string, len(args))
	for i, n := range args {
		tns[i] = cmdutil.NormalizeNumber(n)
	}

	body := map[string]interface{}{
		"TollFreeNumberList": map[string]interface{}{
			"TollFreeNumber": tns,
		},
	}

	var result interface{}
	if err := client.Post(
		fmt.Sprintf("/accounts/%s/tollFreePortingValidations", acctID),
		api.XMLBody{RootElement: "TollFreePortingValidation", Data: body},
		&result,
	); err != nil {
		return portinError(err, "submitting toll-free validation")
	}

	if validateTFWait {
		orderID := digString(result, "OrderId")
		if orderID == "" {
			return fmt.Errorf("validation submitted but response had no OrderId — cannot poll")
		}
		final, err := cmdutil.Poll(cmdutil.PollConfig{
			Interval: 2 * time.Second,
			Timeout:  validateTFTimeout,
			Check: func() (bool, interface{}, error) {
				var r interface{}
				if err := client.Get(
					fmt.Sprintf("/accounts/%s/tollFreePortingValidations/%s", acctID, orderID),
					&r,
				); err != nil {
					return false, nil, portinError(err, "polling validation")
				}
				switch strings.ToUpper(digString(r, "ProcessingStatus")) {
				case "COMPLETE", "FAILED":
					return true, r, nil
				default:
					return false, nil, nil
				}
			},
		})
		if err != nil {
			return err
		}
		result = final
	}

	flat := flattenValidateTFResult(result)
	format, plain := cmdutil.OutputFlags(cmd)
	if plain {
		// Surface non-portable numbers as a hard error so agents don't
		// silently proceed with an order that will never go through.
		if nonPortable := nonPortableEntries(flat); len(nonPortable) > 0 {
			if err := output.StdoutAuto(format, plain, flat); err != nil {
				return err
			}
			return fmt.Errorf("one or more numbers are not portable: %s", summarizeNonPortable(nonPortable))
		}
		return output.StdoutAuto(format, plain, flat)
	}
	return output.StdoutAuto(format, plain, result)
}

// flattenValidateTFResult converts the nested XML response into the v1 plain
// shape: an array of {telephoneNumber, portable, respOrgId, reason} objects.
//
// The XML response groups numbers as portable (under PortableTollFreeNumberList →
// RespOrg → TollFreeNumberList) and not-portable (under various error groupings).
// We collapse both into a single flat array.
func flattenValidateTFResult(result interface{}) []map[string]interface{} {
	out := []map[string]interface{}{}

	// Portable numbers: find every RespOrg entry anywhere in the response,
	// pull its Id and the TNs hanging off it.
	respOrgs := []map[string]interface{}{}
	walkRespOrgs(result, &respOrgs)
	for _, ro := range respOrgs {
		respOrgID := digString(ro, "Id")
		var tns []string
		digAllStrings(ro, "TollFreeNumber", &tns)
		for _, tn := range tns {
			out = append(out, map[string]interface{}{
				"telephoneNumber": cmdutil.NormalizeNumber(tn),
				"portable":        true,
				"respOrgId":       respOrgID,
				"reason":          "",
			})
		}
	}

	// Non-portable numbers: dig into any list group whose name suggests a
	// non-portable category (Spare, Unavailable, Denied, etc.). We treat any
	// number that appears outside PortableTollFreeNumberList as non-portable
	// and capture the surrounding group name as the reason.
	if breakdown := digMap(result, "Breakdown"); breakdown != nil {
		for groupKey, groupVal := range breakdown {
			if groupKey == "PortableTollFreeNumberList" {
				continue
			}
			var tns []string
			digAllStrings(groupVal, "TollFreeNumber", &tns)
			for _, tn := range tns {
				out = append(out, map[string]interface{}{
					"telephoneNumber": cmdutil.NormalizeNumber(tn),
					"portable":        false,
					"respOrgId":       "",
					"reason":          humanReason(groupKey),
				})
			}
		}
	}

	return out
}

func nonPortableEntries(flat []map[string]interface{}) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, e := range flat {
		if p, _ := e["portable"].(bool); !p {
			out = append(out, e)
		}
	}
	return out
}

func summarizeNonPortable(entries []map[string]interface{}) string {
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		tn, _ := e["telephoneNumber"].(string)
		reason, _ := e["reason"].(string)
		parts = append(parts, fmt.Sprintf("%s (%s)", tn, reason))
	}
	return strings.Join(parts, ", ")
}

// walkRespOrgs recurses through the response and appends every RespOrg map
// it finds. A RespOrg map has at least an Id and a TollFreeNumberList. We
// detect it by looking for a map whose key set includes "Id" and
// "TollFreeNumberList".
func walkRespOrgs(v interface{}, out *[]map[string]interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		_, hasID := val["Id"]
		_, hasTFList := val["TollFreeNumberList"]
		if hasID && hasTFList {
			*out = append(*out, val)
			return
		}
		for _, child := range val {
			walkRespOrgs(child, out)
		}
	case []interface{}:
		for _, item := range val {
			walkRespOrgs(item, out)
		}
	}
}

// digMap returns the map at key, or nil if not found.
func digMap(v interface{}, key string) map[string]interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	if child, ok := m[key].(map[string]interface{}); ok {
		return child
	}
	for _, child := range m {
		if got := digMap(child, key); got != nil {
			return got
		}
	}
	return nil
}

func humanReason(groupKey string) string {
	switch strings.ToLower(groupKey) {
	case "sparetollfreenumberlist":
		return "spare — not currently assigned to a RespOrg"
	case "unavailabletollfreenumberlist":
		return "unavailable — reserved by SOMOS"
	case "deniedtollfreenumberlist":
		return "denied — NXX not opened for service"
	case "manuallyportablenumberlist":
		return "manually portable only — requires Bandwidth assistance"
	default:
		return groupKey
	}
}
