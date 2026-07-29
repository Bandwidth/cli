// Package sip implements the `band sip` command group: SIP realms and SIP
// credentials used for SIP trunk digest authentication.
package sip

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/output"
	sipsvc "github.com/Bandwidth/cli/internal/sip"
)

// Cmd is the `band sip` parent command.
//
// The Example block enumerates full leaf command paths with --plain, because a
// caller that only runs `band sip --help` would otherwise see group names and
// have to guess the verbs beneath them.
var Cmd = &cobra.Command{
	Use:   "sip",
	Short: "Manage SIP trunk authentication (realms and credentials)",
	Long: "Manage SIP authentication realms and SIP digest credentials. " +
		"A realm yields the FQDN a SIP peer uses as its outbound address; credentials " +
		"authenticate that peer. Inbound call routing is configured with 'band vcp'.",
	Example: `  # Realms
  band sip realm create --name vapi --default=false --wait --plain
  band sip realm list --plain
  band sip realm get vapi --plain
  band sip realm update vapi --default=true --plain
  band sip realm update vapi --description "Vapi production trunk" --plain
  band sip realm delete vapi --wait --plain

  # Credentials
  band sip credential create --realm vapi --username agent --password-stdin --plain
  band sip credential rotate 870880 --realm vapi --generate-password --plain
  band sip credential list --realm vapi --plain
  band sip credential get 870880 --realm vapi --plain
  band sip credential delete 870880 --realm vapi --plain

  # Status
  band sip status --plain`,
}

// emit is the single output path for every `band sip` command.
//
// Payloads are normalized to generic JSON values (maps/slices/scalars) before
// printing, which does three things:
//
//  1. `--format table` renders. output.printTable has no case for typed structs
//     and falls through to `fmt.Fprintf("%v")`, printing a Go struct dump like
//     `&{1103 vapi vapi-3efeaa.auth.bandwidth.com  false ACTIVE 0}`.
//  2. output.RedactSecrets becomes effective. It only walks
//     map[string]interface{} / []interface{}, so it is structurally inert on
//     typed structs — the redaction net was documentation, not a runtime check.
//     Hash safety still rests primarily on the domain structs carrying no hash
//     fields (see TestDomainStructsCarryNoHashFields); this makes the net real.
//  3. `--plain` is unchanged modulo key order: the same json tags drive both
//     paths.
//
// Precondition: output.FlattenResponse unwraps single-key maps, so a domain
// struct with exactly one field would be flattened down to its bare value. Both
// current domain structs have ≥2 fields; keep it that way.
func emit(format string, plain bool, data interface{}) error {
	normalized, err := normalizeForOutput(data)
	if err != nil {
		return err
	}
	return output.RedactAndPrint(format, plain, normalized)
}

// normalizeForOutput JSON round-trips data into generic values. Every payload
// emit is given is JSON-marshalable by construction (domain structs and plain
// maps), so a failure here means a programming error, not bad user input.
func normalizeForOutput(data interface{}) (interface{}, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("encoding output: %w", err)
	}
	var generic interface{}
	if err := json.Unmarshal(b, &generic); err != nil {
		return nil, fmt.Errorf("normalizing output: %w", err)
	}
	return generic, nil
}

// service builds a SIP service for the active account. It is a package var,
// not a plain func, so tests can substitute a service pointed at a stub server
// — the same seam pattern as cmdutil.VoiceClient.
var service = func(cmd *cobra.Command) (*sipsvc.Service, error) {
	client, acctID, err := cmdutil.DashboardClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return nil, err
	}
	return sipsvc.NewService(client, acctID), nil
}

// faultExit converts documented Bandwidth error codes into actionable messages.
// Exit codes follow from the error TYPE, never from the HTTP status: the same
// logical conflict arrives as 409 (33002/33006/12666), 400 (23022/23026), or a
// 201 body carrying an Errors envelope (23026 on bulk create). Relying on
// APIFault.Unwrap's synthesized *api.APIError would therefore exit 4 for some
// documented conflicts and 1 for others. Every conflict branch below returns
// *cmdutil.ConflictError (exit 4) and keeps the original fault as its cause;
// 33004 stays on FeatureLimitError, which already maps to 4.
func faultExit(err error) error {
	var fault *sipsvc.APIFault
	if !errors.As(err, &fault) {
		return err
	}
	switch fault.Code {
	case "33004":
		return cmdutil.NewFeatureLimit("this account isn't enabled for SIP credentials — contact Bandwidth support to enable SipCredentialSettings", err)
	case "33006":
		return conflict(err, "cannot delete the default realm — make another realm default first: band sip realm update <other-realm> --default=true: %v", err)
	case "12666":
		return conflict(err, "cannot delete this realm while it has SIP credentials — delete them first: band sip credential list --realm <realm>: %v", err)
	case "23022":
		return conflict(err, "realm is not active yet — retry with --wait: %s: %v", fault.Description, err)
	case "33002":
		return conflict(err, "realm already exists: %s: %v", fault.Description, err)
	case "23026":
		return conflict(err, "credential already exists — use --if-not-exists to reuse it, or 'band sip credential rotate <credential-id> --realm <realm>' to change its password: %v", err)
	}
	return err
}

// conflict builds a *cmdutil.ConflictError (exit 4) with a formatted message,
// keeping cause reachable via errors.As so the original API fault's error code
// and status are still inspectable.
func conflict(cause error, format string, args ...interface{}) error {
	return &cmdutil.ConflictError{Message: fmt.Sprintf(format, args...), Cause: cause}
}
