// Package customerprofile implements `band customer-profile`.
//
// Customer profiles are a Numbers v2 resource, but they matter here because a
// profile is a hard prerequisite for 10DLC brand registration: a profile backs
// EXACTLY ONE brand, and reusing one fails with "cannot be assigned to another
// brand". Every new brand needs a freshly created profile.
package customerprofile

import (
	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	cpsvc "github.com/Bandwidth/cli/internal/customerprofile"
)

// Cmd is the `band customer-profile` parent command.
var Cmd = &cobra.Command{
	Use:   "customer-profile",
	Short: "Manage customer profiles",
	Long: `Create and manage customer profiles.

A customer profile is required to register a 10DLC brand, and a profile backs
exactly one brand — create a new profile for each brand you register.

Requires the Customer Profiles Access role. Check with 'band auth status --plain'.`,
}

// service builds a customer-profile service for the active account. A package
// var, not a plain func, so tests can substitute a service pointed at a stub —
// the same seam as cmd/sip's service and cmd/tendlc's.
var service = func(cmd *cobra.Command) (*cpsvc.Service, error) {
	client, acctID, err := cmdutil.PlatformClient(cmdutil.AccountIDFlag(cmd))
	if err != nil {
		return nil, err
	}
	return cpsvc.NewService(client, acctID), nil
}
