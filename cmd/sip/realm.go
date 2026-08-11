package sip

import (
	"github.com/spf13/cobra"

	sipsvc "github.com/Bandwidth/cli/internal/sip"
)

func init() {
	Cmd.AddCommand(realmCmd)
}

var realmCmd = &cobra.Command{
	Use:   "realm",
	Short: "Manage SIP authentication realms",
	Long:  "Create and manage SIP authentication realms. Each realm has a generated FQDN that SIP peers use as their outbound address.",
}

// realmReuseAllowed reports whether an existing realm in this state may be
// returned by --if-not-exists. Terminal-failure and deletion states must not be
// silently reused, and unknown states are never interpreted.
func realmReuseAllowed(status string) bool {
	return status == "ACTIVE" || status == "CREATE_PENDING"
}

// realmStateMatches compares an existing realm against the requested state.
// Only fields the caller specified participate: descriptionSet distinguishes
// "not provided" from "provided as empty".
func realmStateMatches(existing *sipsvc.Realm, wantDefault bool, description string, descriptionSet bool) bool {
	if existing.Default != wantDefault {
		return false
	}
	if descriptionSet && existing.Description != description {
		return false
	}
	return true
}
