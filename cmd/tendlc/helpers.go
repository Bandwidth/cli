package tendlc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
)

// roleGateError wraps a 403 API error with a targeted message based on the
// API response body. The tendlc endpoints return distinct 403 messages:
//   - "is not enabled for the Registration Center" — account feature not enabled
//   - "import customer is not enabled" — account is a direct (not import) customer
//   - "is not enabled on account" — campaign management feature disabled
//   - "does not have access rights" — credential lacks the Campaign Management role
//
// Returns a FeatureLimitError so ExitCodeForError maps these to exit 4
// (escalate to user) rather than exit 2 (re-auth).
func roleGateError(err error, roleName string) error {
	apiErr, ok := err.(*api.APIError)
	if !ok || apiErr.StatusCode != 403 {
		return fmt.Errorf("API request failed: %w", err)
	}

	body := apiErr.Body

	switch {
	case strings.Contains(body, "not enabled for the Registration Center"):
		return cmdutil.NewFeatureLimit("your account is not enabled for the Registration Center.\n"+
			"Contact your Bandwidth account manager to enable the Registration Center feature", err)

	case strings.Contains(body, "import customer is not enabled"):
		return cmdutil.NewFeatureLimit("these commands are for customers who register campaigns through TCR and import\n"+
			"them to Bandwidth. Direct campaign registration through the CLI is coming mid-2026.\n"+
			"In the meantime, use the Bandwidth App or the existing Campaign Management API", err)

	case strings.Contains(body, "direct customer is not enabled"):
		return cmdutil.NewFeatureLimit("these commands are for customers who register campaigns directly through Bandwidth.\n"+
			"Your account is set up as an import customer (campaigns registered through TCR).\n"+
			"Use the import-specific endpoints or contact your Bandwidth account manager", err)

	case strings.Contains(body, "is not enabled on account"):
		return cmdutil.NewFeatureLimit("10DLC campaign management is not enabled on this account.\n"+
			"Contact your Bandwidth account manager to enable messaging and campaign management", err)

	case strings.Contains(body, "does not have access rights"):
		return cmdutil.NewFeatureLimit(fmt.Sprintf("your credentials don't have the %s role.\n"+
			"Contact your Bandwidth account manager to assign the role to your API user", roleName), err)

	default:
		return cmdutil.NewFeatureLimit(fmt.Sprintf("access denied (403): %s\n"+
			"Contact your Bandwidth account manager to check your account configuration", body), err)
	}
}

// extractData unwraps a paginated response to return just the "data" array.
// If the response doesn't match the expected shape, it's returned as-is.
func extractData(result interface{}) interface{} {
	m, ok := result.(map[string]interface{})
	if !ok {
		return result
	}
	if data, exists := m["data"]; exists {
		return data
	}
	return result
}

// filterNumbers applies client-side filtering on the phone numbers list.
// The phoneNumbers endpoint doesn't support server-side filtering on status
// or campaignId, so we filter after fetching.
func filterNumbers(data interface{}, status, campaignID string) interface{} {
	arr, ok := data.([]interface{})
	if !ok {
		return data
	}
	var filtered []interface{}
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if status != "" {
			s, _ := m["status"].(string)
			if !strings.EqualFold(s, status) {
				continue
			}
		}
		if campaignID != "" {
			c, _ := m["campaignId"].(string)
			if !strings.EqualFold(c, campaignID) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	if filtered == nil {
		return []interface{}{}
	}
	return filtered
}

// isNotFound reports whether err is an API 404.
func isNotFound(err error) bool {
	var apiErr *api.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}

// requireConfirm enforces a --confirm gate before any HTTP request is made.
// The message must name the specific consequence — a generic "pass --confirm"
// tells an operator nothing about what they are agreeing to.
func requireConfirm(confirm bool, message string) error {
	if confirm {
		return nil
	}
	return cmdutil.NewFlagError(message)
}

// flagList renders flag names as a "--a, --b, --c" list for error messages.
func flagList(names []string) string {
	out := ""
	for i, n := range names {
		if i > 0 {
			out += ", "
		}
		out += "--" + n
	}
	return out
}

// warnIfTruncated tells the caller on stderr when more records exist than the
// page just returned. stdout stays clean so a pipeline sees only data.
func warnIfTruncated(cmd *cobra.Command, env *api.Envelope, offset, returned int, noun string) {
	if env.Page != nil && env.Page.Truncated(offset+returned) {
		cmd.PrintErrf("showing %d of %d %s; pass --all to fetch every page\n",
			returned, env.Page.TotalElements, noun)
	}
}
