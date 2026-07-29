package sip

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	sipsvc "github.com/Bandwidth/cli/internal/sip"
)

var (
	realmCreateName        string
	realmCreateDescription string
	realmCreateDefault     bool
	realmCreateIfNotExists bool
	realmCreateWait        bool
	realmCreateTimeout     int
)

func init() {
	realmCmd.AddCommand(realmCreateCmd)
	f := realmCreateCmd.Flags()
	f.StringVar(&realmCreateName, "name", "", "Realm name — becomes the first label of the realm FQDN (required)")
	f.StringVar(&realmCreateDescription, "description", "", "Realm description")
	f.BoolVar(&realmCreateDefault, "default", false, "Make this the account's default realm (required — the API rejects creates without it)")
	f.BoolVar(&realmCreateIfNotExists, "if-not-exists", false, "Return the existing realm if one with the same name and state exists")
	f.BoolVar(&realmCreateWait, "wait", false, "Block until the realm reaches ACTIVE")
	f.IntVar(&realmCreateTimeout, "timeout", 60, "Seconds to wait when --wait is set")
	realmCreateCmd.MarkFlagRequired("name")
	realmCreateCmd.MarkFlagRequired("default")
}

var realmCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a SIP authentication realm",
	Long:  "Creates a SIP authentication realm. The response includes the realm's generated FQDN, which is the address a SIP peer uses for outbound calls. Realm creation is asynchronous — use --wait to block until it is ACTIVE.",
	Example: `  # Create a non-default realm and wait for it to become active
  band sip realm create --name vapi --default=false --wait

  # Idempotent create
  band sip realm create --name vapi --default=false --if-not-exists`,
	RunE: runRealmCreate,
}

func runRealmCreate(cmd *cobra.Command, args []string) error {
	if err := sipsvc.ValidateRealmName(realmCreateName); err != nil {
		return err
	}
	svc, err := service(cmd)
	if err != nil {
		return err
	}
	format, plain := cmdutil.OutputFlags(cmd)
	descSet := cmd.Flags().Changed("description")

	if realmCreateIfNotExists {
		realms, err := svc.ListRealms()
		if err != nil {
			return faultExit(err)
		}
		for i := range realms {
			r := realms[i]
			// Case-insensitive per spec line 29: ValidateRealmName accepts
			// uppercase, but the realm name is a DNS label the API may normalize
			// to lowercase. An exact comparison would make --if-not-exists
			// non-idempotent for `--name VAPI` against an existing `vapi`.
			if !strings.EqualFold(r.Name, realmCreateName) {
				continue
			}
			if !realmReuseAllowed(r.Status) {
				return conflict(nil, "realm %q exists but is in state %s — delete it and retry", r.Name, r.Status)
			}
			if !realmStateMatches(&r, realmCreateDefault, realmCreateDescription, descSet) {
				return conflict(nil, "realm %q exists with different settings (default=%v, description=%q) — update it explicitly", r.Name, r.Default, r.Description)
			}
			// --wait must be honored on the reuse path too. realmReuseAllowed
			// admits CREATE_PENDING, so returning here unconditionally would
			// hand an agent exit 0 with status CREATE_PENDING — and AGENTS.md's
			// Timeout Recovery table tells agents that re-running create with
			// --if-not-exists after a --wait timeout is safe, which is exactly
			// how that combination gets used.
			if realmCreateWait && r.Status != "ACTIVE" {
				final, err := waitForRealmActive(svc, r.ID, realmCreateTimeout)
				if err != nil {
					return err
				}
				return emit(format, plain, final)
			}
			return emit(format, plain, r)
		}
	}

	realm, err := svc.CreateRealm(realmCreateName, realmCreateDescription, realmCreateDefault)
	if err != nil {
		return faultExit(err)
	}

	if realmCreateWait && realm.Status != "ACTIVE" {
		final, err := waitForRealmActive(svc, realm.ID, realmCreateTimeout)
		if err != nil {
			return err
		}
		realm = final
	}
	return emit(format, plain, realm)
}

// waitForRealmActive polls until the realm is ACTIVE. Terminal-failure states
// stop the loop immediately rather than burning the whole timeout.
func waitForRealmActive(svc *sipsvc.Service, realmID string, timeoutSeconds int) (*sipsvc.Realm, error) {
	result, err := cmdutil.Poll(cmdutil.PollConfig{
		Interval: 2 * time.Second,
		Timeout:  time.Duration(timeoutSeconds) * time.Second,
		Check: func() (bool, interface{}, error) {
			r, err := svc.GetRealm(realmID)
			if err != nil {
				return false, nil, faultExit(err)
			}
			switch r.Status {
			case "ACTIVE":
				return true, r, nil
			case "CREATE_FAILED", "DELETE_FAILED", "DELETE_PENDING":
				return false, nil, fmt.Errorf("realm %s entered state %s — delete it and retry", r.ID, r.Status)
			case "CREATE_PENDING":
				return false, nil, nil
			default:
				return false, nil, fmt.Errorf("realm %s reported unrecognized state %s", r.ID, r.Status)
			}
		},
	})
	if err != nil {
		return nil, err
	}
	return result.(*sipsvc.Realm), nil
}
