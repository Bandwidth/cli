// Package portin implements the `band portin` command surface for managing
// port-in orders against Bandwidth's Numbers API.
//
// In scope: standalone toll-free portability validation, on-net domestic
// port-ins, automated off-net (Level 3) port-ins, toll-free Phase 1 port-ins
// (gated on TOLL_FREE_AUTOMATION_PHASE_1), bulk port-ins, and lifecycle ops
// (notes, supps, cancel, history, document upload).
//
// Out of scope by design: port-out management, manual toll-free, internal
// toll-free, NASC manual overrides, and international (non-NANP) ports.
// These flows require human action on Bandwidth's side and are documented
// to fail-fast rather than strand a CLI user mid-flow.
package portin

import (
	"github.com/spf13/cobra"

	bulkcmd "github.com/Bandwidth/cli/cmd/portin/bulk"
)

func init() {
	Cmd.AddCommand(bulkcmd.Cmd)
}

// Cmd is the `band portin` parent command.
var Cmd = &cobra.Command{
	Use:   "portin",
	Short: "Manage port-in orders (single, bulk, toll-free)",
	Long: `Create and manage port-in orders against Bandwidth's Numbers API.

Common flows:

  band portin validate-tf +18005551234              # check toll-free portability
  band portin create --numbers +19195551234 ...     # draft a port-in
  band portin upload-loa <order-id> ./loa.pdf       # attach docs
  band portin submit <order-id> --wait              # send to Neustar / SOMOS
  band portin get <order-id>                        # check status

Out of scope: port-out management, manual toll-free ports, internal
toll-free ports, NASC overrides, and international ports. These cannot
be completed end-to-end via the public API and require Bandwidth ops or
the Dashboard.`,
}
