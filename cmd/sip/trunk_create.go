package sip

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/ui"
)

var trunkTransport string

func init() {
	trunkCreateCmd.Flags().StringVar(&trunkTransport, "transport", "", "SIP trunk transport: udp or tls (required; tls also provisions a udp trunk)")
	trunkCmd.AddCommand(trunkCreateCmd)
}

// trunkClient is a swappable ClientFunc seam for tests (see cmdutil.VoiceClient).
var trunkClient cmdutil.ClientFunc = func(accountIDOverride string) (api.Requester, string, error) {
	return cmdutil.BuildAPIClient(accountIDOverride)
}

var trunkCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Opt the account in to SIP trunks",
	Long: `Starts provisioning SIP trunks for the account. This is an opt-in because SIP Auth uses a limited number of ports.

--transport udp provisions a UDP trunk. --transport tls provisions a UDP trunk and a TLS trunk.
An account can have one trunk per transport, so a UDP-only account can still request tls later.

The request is asynchronous: success means it was accepted (state PROVISIONING), not that the
trunks are ready, and there is no status command yet. A 409 means the account already has a trunk
of that transport, a request is already in progress, or account provisioning has not finished (exit 4).`,
	Example: `  band sip trunk create --transport udp
  band sip trunk create --transport tls --plain`,
	RunE: runTrunkCreate,
}

// normalizeTransport upper-cases and validates a --transport value against the wire enum (UDP, TLS).
func normalizeTransport(raw string) (string, error) {
	transport := strings.ToUpper(strings.TrimSpace(raw))
	if transport != "UDP" && transport != "TLS" {
		return "", cmdutil.NewFlagError("--transport is required and must be one of: udp, tls")
	}
	return transport, nil
}

func runTrunkCreate(cmd *cobra.Command, args []string) error {
	transport, err := normalizeTransport(trunkTransport)
	if err != nil {
		return err
	}

	client, acctID, err := trunkClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return err
	}

	var result interface{}
	path := "/accounts/" + url.PathEscape(acctID) + "/sip-trunks"
	if err := client.Post(cmd.Context(), path, map[string]interface{}{"transport": transport}, &result); err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 409 {
			return conflict(err, "SIP trunk request rejected — the account may already have a %s trunk, a request may be in progress, or account provisioning may not be finished; do not retry blindly: %v", transport, err)
		}
		return fmt.Errorf("requesting SIP trunks: %w", err)
	}

	format, plain := cmdutil.OutputFlags(cmd)
	if err := emit(format, plain, result); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr)
	ui.Successf("SIP trunk request accepted")
	ui.Infof("Provisioning is asynchronous and there is no status command yet; this is an acceptance, not a completed trunk.")

	return nil
}
