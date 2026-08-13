package tendlc

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	tendlcsvc "github.com/Bandwidth/cli/internal/tendlc"
)

func init() { Cmd.AddCommand(statusCmd) }

// modeUnknown is returned on every code path. Account mode (direct vs
// import) is a property of how the account is configured with Bandwidth, not
// a runtime-discoverable fact: brand.imported is true on direct AND import
// accounts, and campaign.imported requires an existing campaign plus a
// detail call. Reporting the field as explicitly unknown beats omitting it,
// which invites callers to assume a default.
func modeUnknown() map[string]string {
	return map[string]string{"status": "unknown", "reason": "not_discoverable"}
}

// statusResult maps a probe outcome onto the stable --plain shape.
func statusResult(statusCode int, body string) map[string]any {
	res := map[string]any{"mode": modeUnknown()}
	switch {
	case statusCode >= 200 && statusCode < 300:
		res["access"], res["reason"] = "available", "probe_succeeded"
	case statusCode == 403 && strings.Contains(body, "not enabled for the Registration Center"):
		res["access"], res["reason"] = "unavailable", "registration_center_not_enabled"
	case statusCode == 403 && strings.Contains(body, "is not enabled on account"):
		res["access"], res["reason"] = "unavailable", "campaign_management_not_enabled"
	case statusCode == 403 && strings.Contains(body, "does not have access rights"):
		res["access"], res["reason"] = "unavailable", "role_absent"
	case statusCode == 403:
		// A 403 we don't recognize is still a definite negative — the probe
		// answered the question. Do not guess a specific cause.
		res["access"], res["reason"] = "unavailable", "access_denied"
	default:
		res["access"], res["reason"] = "unknown", "probe_failed"
	}
	return res
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check whether this account can use the 10DLC Registration Center",
	Long: `Probes the Registration Center API to resolve the "unknown" capability reported
by 'band auth status'. Access requires both the Campaign Management role and the
account-level Registration Center feature; only the probe can confirm the latter.

Reports access only. Account mode — whether you register campaigns directly or
import them from TCR — is not probed and cannot be discovered: an account is one
or the other, and that is a property of your Bandwidth setup. If you don't know
which yours is, ask your Bandwidth account contact rather than guessing.`,
	Example: `  band tendlc status --plain`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)
		svc := tendlcsvc.NewService(client, acctID)

		_, probeErr := svc.ListBrands(1, 0, nil)
		if probeErr == nil {
			return output.StdoutAuto(format, plain, statusResult(200, ""))
		}

		var apiErr *api.APIError
		if errors.As(probeErr, &apiErr) {
			res := statusResult(apiErr.StatusCode, apiErr.Body)
			// A 403 is a probe that succeeded in answering the question:
			// this account cannot use the Registration Center. Exit 0 — the
			// command did its job. Anything else is a failure to answer, so
			// emit the "unknown" result for callers parsing stdout, then
			// fall through to the normal non-zero error path.
			if apiErr.StatusCode == 403 {
				return output.StdoutAuto(format, plain, res)
			}
			if emitErr := output.StdoutAuto(format, plain, res); emitErr != nil {
				return emitErr
			}
		}
		return roleGateError(probeErr, "Campaign Management")
	},
}
