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

// service builds a tendlc Service for the active account. It is a package
// var, not a plain func, so tests can substitute a service pointed at a stub
// server — the same seam pattern as cmd/sip's `service` and
// cmdutil.VoiceClient.
var service = func(cmd *cobra.Command) (*tendlcsvc.Service, error) {
	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return nil, err
	}
	return tendlcsvc.NewService(client, acctID), nil
}

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
//
// The 403 branches are checked in a deliberate order — Registration Center,
// then campaign management, then role — because a body could in principle
// contain more than one of these substrings and the first match wins. This
// order reports the most fundamental blocker first: an account-level feature
// gap (Registration Center or campaign management not enabled) is a bigger
// blocker than a missing role on the credential, so it's surfaced ahead of
// role_absent.
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
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		format, plain := cmdutil.OutputFlags(cmd)

		_, probeErr := svc.ListBrands(1, 0, nil)
		if probeErr == nil {
			return output.StdoutAuto(format, plain, statusResult(200, ""))
		}

		// Default to unknown/probe_failed so that EVERY failure path — a
		// recognized 403, an unrecognized 403, a 429/5xx, or a bare transport
		// error (DNS, connection refused, TLS, timeout, context cancellation)
		// — emits a stable JSON document on stdout. A caller parsing stdout
		// must never see an empty body just because the failure didn't happen
		// to arrive wrapped in *api.APIError.
		res := statusResult(0, "")
		var apiErr *api.APIError
		if errors.As(probeErr, &apiErr) {
			res = statusResult(apiErr.StatusCode, apiErr.Body)
			// A 403 is a probe that succeeded in answering the question:
			// this account cannot use the Registration Center. Exit 0 — the
			// command did its job.
			if apiErr.StatusCode == 403 {
				return output.StdoutAuto(format, plain, res)
			}
		}
		// Anything else — 429, 5xx, or a transport error that never made it
		// to an HTTP response — is a failure to answer, not an answer. Emit
		// the result for callers parsing stdout, then fall through to the
		// normal non-zero error path.
		if emitErr := output.StdoutAuto(format, plain, res); emitErr != nil {
			return emitErr
		}
		return roleGateError(probeErr, "Campaign Management")
	},
}
