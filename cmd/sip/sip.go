// Package sip implements the `band sip` command group: SIP realms and SIP
// credentials used for SIP trunk digest authentication.
package sip

import (
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
  band sip realm delete vapi --wait --plain

  # Credentials
  band sip credential create --realm vapi --username agent --password-stdin --plain
  band sip credential rotate 870880 --realm vapi --generate-password --plain
  band sip credential list --realm vapi --plain
  band sip credential get 870880 --realm vapi --plain
  band sip credential delete 870880 --realm vapi --plain`,
}

// emit is the single output path for every `band sip` command. Routing all
// output through one helper means the hash-redaction net cannot be bypassed by
// a future subcommand that prints a raw map — typed domain structs already omit
// hashes, and this catches everything else.
func emit(format string, plain bool, data interface{}) error {
	return output.RedactAndPrint(format, plain, data)
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
// Exit codes follow from the error type via cmdutil.ExitCodeForError.
func faultExit(err error) error {
	var fault *sipsvc.APIFault
	if !errors.As(err, &fault) {
		return err
	}
	switch fault.Code {
	case "33004":
		return cmdutil.NewFeatureLimit("this account isn't enabled for SIP credentials — contact Bandwidth support to enable SipCredentialSettings", err)
	case "33006":
		return fmt.Errorf("cannot delete the default realm — make another realm default first: band sip realm update <other-realm> --default=true: %w", err)
	case "12666":
		return fmt.Errorf("cannot delete this realm while it has SIP credentials — delete them first: band sip credential list --realm <realm>: %w", err)
	case "23022":
		return fmt.Errorf("realm is not active yet — retry with --wait: %s: %w", fault.Description, err)
	case "33002":
		return fmt.Errorf("realm already exists: %s: %w", fault.Description, err)
	case "23026":
		return fmt.Errorf("credential already exists — use --if-not-exists to reuse it, or 'band sip credential rotate <credential-id> --realm <realm>' to change its password: %w", err)
	}
	return err
}
