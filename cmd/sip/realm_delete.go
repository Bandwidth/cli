package sip

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	sipsvc "github.com/Bandwidth/cli/internal/sip"
)

var (
	realmDeleteWait    bool
	realmDeleteTimeout int
)

func init() {
	realmCmd.AddCommand(realmDeleteCmd)
	realmDeleteCmd.Flags().BoolVar(&realmDeleteWait, "wait", false, "Block until the realm is fully deleted")
	realmDeleteCmd.Flags().IntVar(&realmDeleteTimeout, "timeout", 60, "Seconds to wait when --wait is set")
}

var realmDeleteCmd = &cobra.Command{
	Use:     "delete <realm-id-or-name>",
	Short:   "Delete a SIP authentication realm",
	Long:    "Deletes a realm. Deletion is asynchronous (the API returns 202). A realm that still has SIP credentials, or that is the account default, cannot be deleted.",
	Args:    cobra.ExactArgs(1),
	Example: `  band sip realm delete vapi --wait`,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, err := service(cmd)
		if err != nil {
			return err
		}
		// Resolve the ref to the realm's canonical ID before deleting. The
		// command accepts an ID, a short name, or an FQDN, and echoing the input
		// back under "id" made `band sip realm delete vapi --plain` return
		// {"id":"vapi"} — a name in a field an agent reads as an ID. This costs
		// one extra GET on the delete path, which is the price of the output
		// field meaning what it says.
		realm, err := svc.GetRealm(args[0])
		if err != nil {
			return faultExit(err)
		}
		if err := svc.DeleteRealm(realm.ID); err != nil {
			return faultExit(err)
		}
		format, plain := cmdutil.OutputFlags(cmd)
		// A 202 only means the delete was accepted, not that it completed —
		// deleted starts false regardless of --wait and is only promoted to
		// true below once polling confirms the realm is actually gone.
		result := map[string]interface{}{"id": realm.ID, "deleted": false, "accepted": true}

		if realmDeleteWait {
			if _, err := cmdutil.Poll(cmdutil.PollConfig{
				Interval: 2 * time.Second,
				Timeout:  time.Duration(realmDeleteTimeout) * time.Second,
				Check: func() (bool, interface{}, error) {
					// Poll by canonical ID, not by the caller's ref: the name a
					// deleted realm answered to is not guaranteed to keep
					// resolving, and the ID is what the delete was issued against.
					_, err := svc.GetRealm(realm.ID)
					if err == nil {
						return false, nil, nil // still present
					}
					// 404 means the delete completed. Anything else is a real failure.
					var apiErr *api.APIError
					if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
						return true, nil, nil
					}
					var fault *sipsvc.APIFault
					if errors.As(err, &fault) && fault.StatusCode == 404 {
						return true, nil, nil
					}
					return false, nil, faultExit(err)
				},
			}); err != nil {
				return fmt.Errorf("waiting for realm deletion: %w", err)
			}
			result["deleted"] = true
		}
		return emit(format, plain, result)
	},
}
